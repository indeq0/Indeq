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
let statusCheckInterval: ReturnType<typeof setInterval> | null = null;
const POLL_INTERVAL = 1000; // 1 second
const STATUS_CHECK_INTERVAL = 10000; // 10 seconds

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

// Function to periodically check desktop status from server
function startStatusCheck(interval = STATUS_CHECK_INTERVAL) {
  if (!browser) return;
  
  if (statusCheckInterval) clearInterval(statusCheckInterval);
  
  const checkDesktopStatus = async () => {
    try {
      // Reuse the existing endpoint instead of creating a new one
      const response = await fetch('/api/desktop-stats');
      if (response.ok) {
        const data = await response.json();
        // Only update the isOnline status
        desktopIntegration.update(current => ({
          ...current,
          isOnline: data.isOnline,
          isCrawling: data.isCrawling
        }));
      }
    } catch (err) {
      console.error('Error checking desktop status:', err);
    }
  };
  
  // Check immediately
  checkDesktopStatus();
  
  // Then set up interval
  statusCheckInterval = setInterval(checkDesktopStatus, interval);
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

function stopStatusCheck() {
  if (statusCheckInterval) {
    clearInterval(statusCheckInterval);
    statusCheckInterval = null;
  }
}

export { 
  desktopIntegration, 
  startPolling, 
  stopPolling, 
  initialize, 
  startStatusCheck, 
  stopStatusCheck 
};