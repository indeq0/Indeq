import { fail } from '@sveltejs/kit';
import type { Actions } from './$types';
import { PUBLIC_GO_BACKEND_URL } from '$env/static/public';

export const actions: Actions = {
	default: async ({ request }) => {
    const formData = await request.formData();
    const betaCode = formData.get('betaCode') as string;
    const email = formData.get('email') as string;
    if (!email || !betaCode) {
      return fail(400, { error: 'Missing email or beta code.' });
    }

    try {      
      const res = await fetch(`${PUBLIC_GO_BACKEND_URL}/api/validate-beta-code`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ beta_code: betaCode, email })
      });

      const data = await res.json();
      if (!res.ok) {
        return fail(res.status, { error: data.message ?? 'Access denied.' });
      }
      if (!data.success) {
        return fail(res.status, { error: data.message ?? 'Access denied.' });
      }

      return { success: true };
    } catch (err) {
      return fail(500, { error: 'Something went wrong. Please try again.' });
    }
  }
};