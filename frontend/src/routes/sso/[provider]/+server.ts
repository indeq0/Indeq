import { redirect } from '@sveltejs/kit';
import {
  GOOGLE_SSO_CLIENT_ID,
  GOOGLE_AUTH_URL,
  GOOGLE_SSO_SCOPES,
  GOOGLE_SSO_REDIRECT_URI,
  GO_BACKEND_URL,
} from '$env/static/private';

const OAUTH_CONFIG = {
  GOOGLE: {
    clientId: GOOGLE_SSO_CLIENT_ID,
    authUrl: GOOGLE_AUTH_URL,
    scopes: GOOGLE_SSO_SCOPES?.split(' '),
    redirectUri: GOOGLE_SSO_REDIRECT_URI
  }
} as const;

export const GET = async ({ params, fetch, cookies }) => {
  const provider = params.provider as keyof typeof OAUTH_CONFIG;
  const config = OAUTH_CONFIG[provider];
  if (!config) {
    return new Response('Invalid provider', { status: 400 });
  }

  // Get the OAuth URL from the backend for signin/jwt retrieval
  const res = await fetch(`${GO_BACKEND_URL}/api/ssooauth`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ provider })
  });

  if (!res.ok) {
    const errorBody = await res.json();
    return new Response(errorBody.message || 'Failed to get OAuth URL', { status: res.status });
  }

    const data = await res.json();
    const oauthUrl = data.url;
    redirect(302, oauthUrl);
};
