import type { Actions } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { redirect, fail } from '@sveltejs/kit';
import { GO_BACKEND_URL } from '$env/static/private';

/* ──────────────────────────────────────────
   1.  LOAD  – runs on first navigation/refresh
   ────────────────────────────────────────── */
export const load: PageServerLoad = async ({ url, cookies }) => {
  const type = url.searchParams.get('type');

  // guard against bogus or missing ?type
  if (type !== 'register' && type !== 'forgot') {
    throw redirect(303, '/register');
  }

  /* ---------- token check ---------- */
  const token =
    type === 'register'
      ? cookies.get('pendingRegisterToken') || cookies.get('jwt') // register also OK if already logged in
      : cookies.get('pendingForgotToken');

  const expired = !token;

  return { context: type, expired };        // <─ pass flag to the page
};

/* ──────────────────────────────────────────
   2. ACTION  – handles form submits
   ────────────────────────────────────────── */
export const actions = {
  default: async ({ request, cookies }) => {
    const data = await request.formData();
    const type = data.get('type');
    const code = data.get('code');
    const resend = data.get('resend');

    if (type !== 'register' && type !== 'forgot') {
      return fail(400, { error: 'Invalid type' });
    }

    const token =
      cookies.get('pendingRegisterToken') ||
      cookies.get('pendingForgotToken');

    /* ---------- token missing ⇒ session expired ---------- */
    if (!token) {
      return fail(419, {             // 419 = “authentication timeout”
        expired: true,
        context: type
      });
    }

    /* ---------- RESEND ---------- */
    if (resend === 'true') {
      const r = await fetch(`${GO_BACKEND_URL}/api/resend-otp`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type, token })
      });

      if (!r.ok) {
        return fail(r.status, { error: await r.text() });
      }

      const json = await r.json();
      if (!json.success) {
        return fail(400, { error: json.message });
      }

      return {
        success: true,
        message: 'A new verification code has been sent to your email.'
      };
    }

    /* ---------- VERIFY ---------- */
    const v = await fetch(`${GO_BACKEND_URL}/api/verify-otp`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type, code, token })
    });

    if (!v.ok) {
      return fail(v.status, { error: await v.text() });
    }

    const json = await v.json();
    if (!json.success) {
      return fail(400, { error: json.error });
    }

    // registration succeeds ⇒ issue JWT & clear pending token
    if (type === 'register') {
      cookies.set('jwt', json.token, {
        path: '/',
        httpOnly: true,
        secure: true,
        sameSite: 'lax',
        maxAge: 60 * 60 * 24 // 1 day
      });
      cookies.delete('pendingRegisterToken', { path: '/' });
    }

    return { success: true, verifiedType: type };
  }
} satisfies Actions;
