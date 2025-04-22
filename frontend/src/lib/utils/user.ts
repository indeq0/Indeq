import { userStore } from '$lib/stores/userStore';

/**
 * Generates a random avatar number for new users
 * @returns A number between 1 and 8 (inclusive)
 */
export function generateRandomAvatar(): number {
  return Math.floor(Math.random() * 8) + 1;
}

/**
 * Formats user data from API response
 * @param userData Raw user data from API
 * @returns Formatted user object
 */
export function formatUserData(userData: any) {
  return {
    email: userData.email,
    name: userData.name,
    avatar: userData.avatar || generateRandomAvatar(),
    alias: userData.alias || ''
  };
}

/**
 * Fetches current user data from the API and updates the user store
 * @returns Promise that resolves when the user data has been fetched and stored
 */
export async function fetchAndStoreUserData(): Promise<void> {
  try {
    const res = await fetch('/api/me');
    if (res.ok) {
      const userData = await res.json();
      userStore.setUser(formatUserData(userData));
    }
  } catch (error) {
    console.error('Error fetching user data:', error);
  }
}

