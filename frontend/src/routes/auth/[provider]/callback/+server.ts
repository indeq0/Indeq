import { redirect } from '@sveltejs/kit';
import { GO_BACKEND_URL } from '$env/static/private';

export const GET = async ({ params, url, cookies }) => {
  const provider = params.provider;
  const code = url.searchParams.get('code');
  const state = url.searchParams.get('state');

  // if valid, exchange code for token
  if (code && state && provider) {
    const authData = { Provider: provider, AuthCode: code, State: state };
    const token = cookies.get('jwt');
    if (!token) {
      // console.error('No JWT token found, user is not authenticated');
      throw redirect(302, '/login');
    }

    try {
      const response = await fetch(`${GO_BACKEND_URL}/api/connect`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`
        },
        body: JSON.stringify(authData)
      });

      if (!response.ok) {
        // const errorText = await response.text();
        // console.error('Error connecting to provider:', errorText);
        throw redirect(
          302,
          `/profile/integration?status=error&action=connect&provider=${encodeURIComponent(provider)}`
        );
      }

      const data = await response.json();

      if (data.error) {
        throw redirect(
          302,
          `/profile/integration?status=error&action=connect&provider=${encodeURIComponent(provider)}`
        );
      }
    } catch (error) {
      if (!(error instanceof redirect)) {
        // console.error('Integration error:', error);
        throw redirect(
          302,
          `/profile/integration?status=error&action=connect&provider=${encodeURIComponent(provider)}`
        );
      }
      throw error;
    }

    throw redirect(
      302,
      `/profile/integration?status=success&action=connect&provider=${encodeURIComponent(provider)}`
    );
  }

  throw redirect(302, '/profile/integration');
};
