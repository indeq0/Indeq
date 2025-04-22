import { userStore } from '$lib/stores/userStore';

/**
 * Generates a random avatar number for new users
 * @returns A number between 1 and 6 (inclusive)
 */
export function generateRandomAvatar(): number {
  return Math.floor(Math.random() * 6) + 1;
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
    avatar: userData.avatar_num || generateRandomAvatar(),
    alias: userData.alias || userData.name[0]
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

/**
 * Normalizes an avatar number to ensure it's within the valid range (1-6)
 * @param avatarNumber The avatar number to normalize
 * @returns A number between 1 and 6 (inclusive)
 */
export function normalizeAvatarNumber(avatarNumber: number | undefined | null): number {
  const avatar = avatarNumber || 1;
  return avatar > 6 ? (avatar % 6 || 6) : Math.max(1, avatar);
}

