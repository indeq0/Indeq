import { fail } from '@sveltejs/kit';
import type { Actions } from './$types';
import { PUBLIC_GO_BACKEND_URL } from '$env/static/public';

export const actions: Actions = {
	default: async ({ request }) => {
    const formData = await request.formData();
    const email = formData.get('email') as string;
    const betaCode = formData.get('betaCode') as string;

    if (!email || !betaCode) {
      return fail(400, { error: 'Missing email or beta code.' });
    }

    try {
      const res = await fetch(`${PUBLIC_GO_BACKEND_URL}/api/validate-beta-code`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ email, betaCode })
      });

      if (!res.ok) {
        const { message } = await res.json();
        return fail(res.status, { error: message ?? 'Access denied.' });
      }

      return { success: true };
    } catch (err) {
      console.error('Validation failed:', err);
      return fail(500, { error: 'Something went wrong. Please try again.' });
    }
  }
};