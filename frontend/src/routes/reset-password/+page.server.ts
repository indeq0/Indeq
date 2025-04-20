import type { Actions } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";
import { fail, redirect } from "@sveltejs/kit";
import { GO_BACKEND_URL } from "$env/static/private";

export const load: PageServerLoad = async ({ cookies }) => {
  const pendingForgot = cookies.get('pendingForgotToken');

  if (!pendingForgot) {
    throw redirect(303, '/forgot-password');
  }

  return { context: 'reset' };
};

export const actions: Actions = {
  default: async ({ request, cookies }) => {
    const data = await request.formData();
    const password = data.get('password');
    const token = cookies.get('pendingForgotToken');

    if (!token) {
      return fail(400, { error: 'Something went wrong. Please try again.' });
    }

    // Call backend to update password
    const res = await fetch(`${GO_BACKEND_URL}/api/reset-password`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password, token })
    });

    if (!res.ok) {
      const msg = await res.text();
      return fail(res.status, { error: msg });
    }

    const response = await res.json();

    if (!response.success) {
      return fail(400, { error: response.error || 'Failed to reset password' });
    }

    // Clear the reset session
    cookies.delete('pendingForgotToken', { path: '/' });

    return { success: true };
  }
};
