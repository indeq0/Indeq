import { writable } from 'svelte/store';
import { browser } from '$app/environment';
import type { DesktopIntegration } from '$lib/types/desktopIntegration';

// Create a writable store with initial empty state
const desktopIntegration = writable<DesktopIntegration>({
  crawledFiles: 0,
  totalFiles: 0,
  isOnline: false,
  isCrawling: false
});

// Polling internals
let pollingInterval: ReturnType<typeof setInterval> | null = null;
const POLL_INTERVAL = 500; // 5 seconds

// Function to start polling the local endpoint (not the GO backend directly)
function startPolling(interval = POLL_INTERVAL) {
  if (!browser) return;
  
  if (pollingInterval) clearInterval(pollingInterval);
  
  const fetchDesktopStats = async () => {
    try {
      const response = await fetch('/api/desktop-stats');
      if (response.ok) {
        const data = await response.json();
        desktopIntegration.set(data);
      }
    } catch (err) {
      console.error('Error polling desktop stats:', err);
    }
  };
  
  // Fetch immediately
  fetchDesktopStats();
  
  // Then set up interval
  pollingInterval = setInterval(fetchDesktopStats, interval);
}

// Initialize with initial data
function initialize(initialData: DesktopIntegration) {
  desktopIntegration.set(initialData);
}

function stopPolling() {
  if (pollingInterval) {
    clearInterval(pollingInterval);
    pollingInterval = null;
  }
}

export { desktopIntegration, startPolling, stopPolling, initialize };