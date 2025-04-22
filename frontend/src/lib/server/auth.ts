import { GO_BACKEND_URL } from '$env/static/private';

export async function verifyToken(token: string) {
  try {
    const res = await fetch(`${GO_BACKEND_URL}/api/verify`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`
      },
      body: JSON.stringify({})
    });

    if (!res.ok) {
      throw new Error('Failed to verify token');
    }

    const response = await res.json();
    return response.valid;
  } catch (error) {
    return false; // Token is invalid
  }
}
