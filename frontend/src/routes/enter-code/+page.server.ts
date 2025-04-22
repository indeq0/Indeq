import type { Actions } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { redirect, fail } from '@sveltejs/kit';
import { GO_BACKEND_URL } from '$env/static/private';

export const load: PageServerLoad = async ({ url, cookies }) => {
  const type = url.searchParams.get('type');

  if (type !== 'register' && type !== 'forgot') {
    throw redirect(303, '/register');
  }

  const token =
    type === 'register'
      ? cookies.get('pendingRegisterToken') || cookies.get('jwt')
      : cookies.get('pendingForgotToken');

  const expired = !token;

  return { context: type, expired };
};

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

    if (!token) {
      return fail(419, {
        expired: true,
        context: type
      });
    }

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

    if (type === 'register') {
      cookies.set('jwt', json.token, {
        path: '/',
        httpOnly: true,
        secure: true,
        sameSite: 'lax',
        maxAge: 60 * 60 * 24 // 1 day
      });
      cookies.delete('pendingRegisterToken', { path: '/' });
      
      // Fetch user data after successful registration
      try {
        const userRes = await fetch(`${GO_BACKEND_URL}/api/me`, {
          headers: {
            'Authorization': `Bearer ${json.token}`
          }
        });
        
        if (userRes.ok) {
          const userData = await userRes.json();
          return { 
            success: true, 
            verifiedType: type, 
            user: {
              email: userData.email,
              name: userData.name,
              avatar: userData.avatar || '',
              alias: userData.alias || ''
            }
          };
        }
      } catch (error) {
        console.error('Error fetching user data:', error);
      }
    }

    return { success: true, verifiedType: type };
  }
} satisfies Actions;
