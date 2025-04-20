import { fail, redirect, type Actions } from '@sveltejs/kit';
import { GO_BACKEND_URL } from '$env/static/private';

export const actions: Actions = {
  default: async ({ request, cookies }) => {
    const data = await request.formData();
    const email = data.get('email');

    if (!email || typeof email !== 'string') {
      return fail(400, { error: 'Invalid email' });
    }

    const forgotPasswordRes = await fetch(`${GO_BACKEND_URL}/api/forgot-password`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ email })
    })

    if (!forgotPasswordRes.ok) {
      const msg = await forgotPasswordRes.text();
      return fail(400, { error: msg });
    }

    const response = await forgotPasswordRes.json();

    if (!response.success) {
      return fail(400, { error: response.error || 'An unknown error occurred' });
    }

    const token = response.token;

    cookies.set(
      'pendingForgotToken',
      token,
      {
        httpOnly: true,
        secure: true,
        path: '/',
        sameSite: 'lax',
        maxAge: 60 * 5 // 5 minutes
      }
    );

    throw redirect(303, '/enter-code?type=forgot');
  }
};
