import { fail, type Cookies } from '@sveltejs/kit';
import type { Actions } from './$types';
import { GO_BACKEND_URL } from '$env/static/private';

const login = async (email: string, password: string, cookies: Cookies) => {
  const loginRes = await fetch(`${GO_BACKEND_URL}/api/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: email, password: password })
  });

  if (!loginRes.ok) {
    const msg = await loginRes.text();
    return fail(loginRes.status, { error: msg });
  }

  const response = await loginRes.json();
  cookies.set('jwt', response.token, {
    httpOnly: true,
    secure: true,
    path: '/',
    maxAge: 60 * 60 * 24,
    sameSite: 'lax'
  });

  cookies.set('registering', 'true', {
    httpOnly: true,
    secure: true,
    path: '/',
    maxAge: 5,
    sameSite: 'lax'
  });

  cookies.set('user_created', 'true', {
    httpOnly: true,
    secure: true,
    path: '/',
    maxAge: 5,
    sameSite: 'lax'
  });
};

export const actions = {
  default: async ({ request, cookies }) => {
    const data = await request.formData();
    const email = data.get('email');
    const password = data.get('password');
    const name = data.get('name');

    // Basic validation
    if (!email || !password || !name) {
      return fail(400, {
        error: 'Email, password and name are required',
        email: email?.toString(),
        name: name?.toString()
      });
    }

    const registerRes = await fetch(`${GO_BACKEND_URL}/api/register`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ email: email, name: name, password: password })
    });

    if (!registerRes.ok) {
      const msg = await registerRes.text();

      // Return an error to the page to display
      return fail(registerRes.status, { error: msg });
    }

    const response = await registerRes.json();

    if (response.success) {
      await login(email.toString(), password.toString(), cookies);
      return { success: true };
    } else {
      return fail(400, { error: response.error });
    }
  }
} satisfies Actions;
