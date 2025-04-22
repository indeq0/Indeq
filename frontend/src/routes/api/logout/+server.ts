import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { userStore } from '$lib/stores/userStore';

export const POST: RequestHandler = async ({ cookies }) => {
  
  // Clear the JWT cookie
  cookies.delete('jwt', {
    path: '/',
  });

  userStore.clearUser();
  
  return json({ success: true });
}; 