import { writable, derived } from 'svelte/store';
import { browser } from '$app/environment';

/**
 * An interface defining the shape of your user data.
 * Based on the existing User interface structure.
 */
export interface User {
	email: string;
	avatar: number;
  name: string;
  alias: string;
}

/**
 * This interface represents the overall state our store will manage.
 */
interface UserState {
  user: User | null;
  isLoading: boolean;
  isLoggedIn: boolean;
  initialized: boolean;
}

/**
 * Creates a custom user store tailored for server-side JWT authentication.
 * The actual auth verification happens on the server, and this store handles
 * the client-side state management of the authenticated user.
 */
function createUserStore() {
  // Initialize the store with default values
  const { subscribe, set, update } = writable<UserState>({
    user: null,
    isLoading: false,
    isLoggedIn: false,
    initialized: false
  });

  // If browser, check if we have stored user data in localStorage
  if (browser) {
    try {
      const storedUser = localStorage.getItem('user');
      if (storedUser) {
        const userData = JSON.parse(storedUser);
        set({
          user: userData,
          isLoading: false,
          isLoggedIn: true,
          initialized: true
        });
      } else {
        set({
          user: null,
          isLoading: false,
          isLoggedIn: false,
          initialized: true
        });
      }
    } catch (e) {
      // If there's an error, just mark as initialized
      update(state => ({ ...state, initialized: true }));
      console.error('Error initializing user store:', e);
    }
  }

  return {
    subscribe,

    /**
     * Set user data after successful server-side authentication
     * This should be called after the server validates the JWT token
     * and returns the user data
     */
    setUser: (userData: User) => {
      if (browser) {
        localStorage.setItem('user', JSON.stringify(userData));
      }
      
      set({
        user: userData,
        isLoading: false,
        isLoggedIn: true,
        initialized: true
      });
    },

    /**
     * Clear user data after logout
     * This should be called after the server handles logout and invalidates
     * the JWT token (typically clearing the cookie on the server side)
     */
    clearUser: () => {
      if (browser) {
        localStorage.removeItem('user');
      }
      
      set({
        user: null,
        isLoading: false,
        isLoggedIn: false,
        initialized: true
      });
    },

    /**
     * Update user data
     * For use when user details change but the authentication status remains the same
     */
    updateUser: (partialUser: Partial<User>) => {
      update((state) => {
        if (!state.user) {
          return state;
        }
        
        const updatedUser = {
          ...state.user,
          ...partialUser
        };
        
        if (browser) {
          localStorage.setItem('user', JSON.stringify(updatedUser));
        }
        
        return {
          ...state,
          user: updatedUser
        };
      });
    },

    /**
     * Set loading state during authentication operations
     */
    setLoading: (isLoading: boolean) => {
      update(state => ({ ...state, isLoading }));
    }
  };
}

// Export a single instance of this store for the entire app to use
export const userStore = createUserStore();

// Derived stores for convenience
export const user = derived(userStore, $store => $store.user);
export const isLoggedIn = derived(userStore, $store => $store.isLoggedIn);
export const isLoading = derived(userStore, $store => $store.isLoading);
export const initialized = derived(userStore, $store => $store.initialized);
