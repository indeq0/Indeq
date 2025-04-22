import { redirect } from '@sveltejs/kit';
import {
  GOOGLE_CLIENT_ID,
  GOOGLE_AUTH_URL,
  GOOGLE_SCOPES,
  GOOGLE_REDIRECT_URI,
  MICROSOFT_CLIENT_ID,
  MICROSOFT_AUTH_URL,
  MICROSOFT_SCOPES,
  MICROSOFT_REDIRECT_URI,
  NOTION_CLIENT_ID,
  NOTION_AUTH_URL,
  GO_BACKEND_URL
} from '$env/static/private';

const OAUTH_CONFIG = {
  GOOGLE: {
    clientId: GOOGLE_CLIENT_ID,
    authUrl: GOOGLE_AUTH_URL,
    scopes: GOOGLE_SCOPES?.split(' '),
    redirectUri: GOOGLE_REDIRECT_URI
  },
  MICROSOFT: {
    clientId: MICROSOFT_CLIENT_ID,
    authUrl: MICROSOFT_AUTH_URL,
    scopes: MICROSOFT_SCOPES?.split(' '),
    redirectUri: MICROSOFT_REDIRECT_URI
  },
  NOTION: {
    clientId: NOTION_CLIENT_ID,
    authUrl: NOTION_AUTH_URL
  }
} as const;

export const GET = async ({ params, fetch, cookies }) => {
  const provider = params.provider as keyof typeof OAUTH_CONFIG;
  const config = OAUTH_CONFIG[provider];
  const token = cookies.get('jwt');
  if (!config) {
    return new Response('Invalid provider', { status: 400 });
  }

  const res = await fetch(`${GO_BACKEND_URL}/api/oauth`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`
    },
    body: JSON.stringify({ provider })
  });

  if (!res.ok) {
    const errorBody = await res.json();
    return new Response(errorBody.message || 'Failed to get OAuth URL', { status: res.status });
  }

  const data = await res.json();
  const oauthUrl = data.url;
  throw redirect(302, oauthUrl);
};
