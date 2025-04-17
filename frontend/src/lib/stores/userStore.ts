import { writable } from 'svelte/store';

/**
 * An interface defining the shape of your user data.
 * Add or remove fields as needed.
 */
export interface User {
	id: number | null;
	username: string;
	email: string;
	avatar: string;
	// Add more fields if you wish, e.g. avatarUrl, role, etc.
}

/**
 * This interface represents the overall state our store will manage.
 */
interface UserState {
  user: User | null;
  isLoggedIn: boolean;
}

/**
 * Creates a custom user store with helper methods for login, logout, and updates.
 */
function createUserStore() {
  // Initialize the store with default values
  const { subscribe, set, update } = writable<UserState>({
    user: null,
    isLoggedIn: false
  });

  return {
    subscribe,

    /**
     * Log the user in by setting the user object and marking isLoggedIn = true.
     * Example usage: userStore.login({ id: 1, username: 'alice', email: 'alice@example.com' });
     */
    login: (userData: User) => {
      set({
        user: userData,
        isLoggedIn: true
      });
    },

    /**
     * Log the user out, clearing all user data.
     */
    logout: () => {
      set({
        user: null,
        isLoggedIn: false
      });
    },

    /**
     * Update selected fields of the current user without overwriting everything.
     * For example, updating their email or username.
     */
    updateUser: (partialUser: Partial<User>) => {
      update((current) => {
        if (!current.user) {
          // If no user is currently set, do nothing
          return current;
        }
        return {
          ...current,
          user: {
            ...current.user,
            ...partialUser
          }
        };
      });
    }
  };
}

// Export a single instance of this store for the entire app to use
export const userStore = createUserStore();
