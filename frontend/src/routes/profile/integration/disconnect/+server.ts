import { error, redirect } from '@sveltejs/kit';
import { GO_BACKEND_URL } from '$env/static/private';

export const GET = async ({ url, cookies, fetch }) => {
  const provider = url.searchParams.get('provider');
  if (!provider) {
    throw error(400, 'Provider is required');
  }

  const token = cookies.get('jwt');
  if (!token) {
    throw redirect(302, '/login');
  }
  let response;
  try {
    response = await fetch(`${GO_BACKEND_URL}/api/disconnect`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ provider })
    });
  } catch (err) {
    console.error('Error disconnecting:', err);
    throw redirect(
      302,
      `/profile/integration?status=error&action=disconnect&provider=${encodeURIComponent(provider)}`
    );
  }

  if (!response.ok) {
    throw redirect(
      302,
      `/profile/integration?status=error&action=disconnect&provider=${encodeURIComponent(provider)}`
    );
  }

  throw redirect(
    302,
    `/profile/integration?status=success&action=disconnect&provider=${encodeURIComponent(provider)}`
  );
};
