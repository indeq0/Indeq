import { fail } from '@sveltejs/kit';
import type { Actions } from './$types';
import { GO_BACKEND_URL } from '$env/static/private';

export const actions = {
  default: async ({ request, cookies }) => {
    const data = await request.formData();
    const email = data.get('email');
    const password = data.get('password');

    // Basic validation
    if (!email || !password) {
      return fail(400, {
        error: 'Email and password are required',
        email: email?.toString()
      });
    }

    try {
      const res = await fetch(`${GO_BACKEND_URL}/api/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email, password: password })
      });

      if (!res.ok) {
        const msg = await res.text();

        // Return an error to the page to display
        return fail(res.status, { error: msg });
      }

      const response = await res.json();

      // Store JWT token in an HTTP-only cookie
      cookies.set('jwt', response.token, {
        httpOnly: true, // Prevent client-side access
        secure: true, // Only send over HTTPS
        path: '/', // Accessible across the entire app
        maxAge: 60 * 60 * 24, // 1 day
        sameSite: 'lax'
      });

      if (response.error == null || response.error === '') {
        return { success: true };
      } else {
        return fail(400, { error: response.error });
      }
    } catch (error) {
      return fail(400, {
        error: 'Invalid credentials',
        email: email?.toString()
      });
    }
  }
} satisfies Actions;
