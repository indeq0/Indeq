import type { PageServerLoad } from './$types';
import { redirect, error } from '@sveltejs/kit';
import { GO_BACKEND_URL } from '$env/static/private';
import type { DesktopIntegration } from '$lib/types/desktopIntegration';
import { LogInIcon } from 'svelte-feather-icons';

export const load: PageServerLoad = async ({ cookies, fetch }) => {
  const session = cookies.get('jwt');
  const userCreated = cookies.get('user_created');
  const redirectedFrom = cookies.get('redirected_from');
  const registering = cookies.get('registering');
  const loggingIn = cookies.get('loggingIn');

  // If user was just created, clear the cookie after 5 seconds
  if (userCreated) {
    cookies.set('user_created', '', {
      path: '/',
      maxAge: 5 // 5 seconds
    });
  }

  // Clean up the registering cookie
  if (registering) {
    cookies.set('registering', '', {
      path: '/',
      maxAge: 5
    });
  }

  if (loggingIn) {
    cookies.set('loggingIn', '', {
      path: '/',
      maxAge: 5
    });
  }

  if (!session) {
    // No user, redirect to login
    throw redirect(302, '/login');
  }

  const integrations = await fetch(`${GO_BACKEND_URL}/api/integrations`, {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${session}`
    }
  });

  const desktopIntegration = await fetch(`${GO_BACKEND_URL}/api/desktop_stats`, {
    method: 'GET',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${session}`
    }
  });

  let integrationData: { providers?: string[] };
  let desktopIntegrationData: DesktopIntegration;

  try {
    integrationData = await integrations.json();
    desktopIntegrationData = await desktopIntegration.json();
  } catch (err) {
    throw error(500, ' to fetch integrations');
  }

  const providers = integrationData.providers ?? [];

  return {
    integrations: providers,
    desktopInfo: desktopIntegrationData,
    userCreated: userCreated,
    redirectedFrom: redirectedFrom,
    registering: registering,
    loggingIn: loggingIn
  };
};
