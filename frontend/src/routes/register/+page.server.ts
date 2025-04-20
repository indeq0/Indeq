import { fail, redirect } from '@sveltejs/kit';
import type { Actions } from './$types';
import { GO_BACKEND_URL } from '$env/static/private';

export const actions = {
  default: async ({ request, cookies }) => {
    const data = await request.formData();
    const email = data.get('email');
    const name = data.get('name');
    const password = data.get('password');

    if (!email || !name || !password) {
      return fail(400, {
        error: 'Email, name, and password are required',
        email: email?.toString(),
        name: name?.toString()
      });
    }

    const registerRes = await fetch(`${GO_BACKEND_URL}/api/register`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ email, name, password })
    });

    if (!registerRes.ok) {
      const msg = await registerRes.text();
      return fail(registerRes.status, { error: msg });
    }

    const response = await registerRes.json();
    if (!response.success) {
      return fail(400, { error: response.error });
    }

    cookies.set('pendingRegisterToken', response.token, {
      path: '/',
      httpOnly: true,
      secure: true,
      sameSite: 'lax',
      maxAge: 300, // 5 minutes
    });
    return { success: true };
  }
} satisfies Actions;