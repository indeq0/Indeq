import type { LayoutServerLoad } from './$types';
import { GO_BACKEND_URL } from '$env/static/private';
import type { DesktopIntegration } from '$lib/types/desktopIntegration';

export const load: LayoutServerLoad = async ({ cookies, fetch }) => {
  const session = cookies.get('jwt');
  
  if (!session) {
    // Return empty desktop info if not logged in
    return {
      desktopInfo: {
        crawledFiles: 0,
        totalFiles: 0,
        isOnline: false,
        isCrawling: false
      }
    };
  }

  try {
    const desktopIntegration = await fetch(`${GO_BACKEND_URL}/api/desktop_stats`, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${session}`
      }
    });

    const desktopIntegrationData: DesktopIntegration = await desktopIntegration.json();

    return {
      desktopInfo: desktopIntegrationData
    };
  } catch (error) {
    console.error('Failed to fetch desktop integration:', error);
    
    // Return default data if fetch fails
    return {
      desktopInfo: {
        crawledFiles: 0,
        totalFiles: 0,
        isOnline: false,
        isCrawling: false
      }
    };
  }
}; 