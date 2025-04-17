import { writable } from 'svelte/store';
import { browser } from '$app/environment';

const SIDEBAR_KEY = 'sidebar_expanded';

const getInitialState = (): boolean => {
  if (!browser) return true;
  const stored = localStorage.getItem(SIDEBAR_KEY);
  return stored !== null ? stored === 'true' : true;
};

export const sidebarExpanded = writable<boolean>(getInitialState());

if (browser) {
  sidebarExpanded.subscribe(value => {
    localStorage.setItem(SIDEBAR_KEY, String(value));
  });
}

export const toggleSidebar = () => sidebarExpanded.update(value => !value);