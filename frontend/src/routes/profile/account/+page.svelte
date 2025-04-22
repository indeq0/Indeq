<script lang="ts">
  import { userStore, type User } from '$lib/stores/userStore';
  import { fetchAndStoreUserData, normalizeAvatarNumber } from '$lib/utils/user';
  import { onMount } from 'svelte';
  import { toast } from 'svelte-sonner';
  import { fade, slide } from 'svelte/transition';

  $: user = $userStore.user;
  $: userName = user?.name || '';
  $: userAlias = user?.alias || '';
  $: hasNameChanges = user && (userName !== user.name || userAlias !== user.alias);

  // Profile gradient variables
  let isGradientDropdownOpen = false;
  let selectedGradient = "";

  onMount(() => {
    // Ensure avatar is within the valid range (1-6)
    const normalizedAvatar = normalizeAvatarNumber(user?.avatar);
    selectedGradient = `/gradients/gradient-${normalizedAvatar}.png`;
  });
  
  const gradients = [
    "/gradients/gradient-1.png",
    "/gradients/gradient-2.png",
    "/gradients/gradient-3.png",
    "/gradients/gradient-4.png",
    "/gradients/gradient-5.png",
    "/gradients/gradient-6.png",
  ];

  async function updateUser(user: User) {
    await fetch('/api/me', {
      method: 'POST',
      body: JSON.stringify(user)
    });
  }

  async function saveProfileGradient(gradient: string) {
    if (!user) return;
    
    const avatarNumber = parseInt(gradient.split('-')[1].replace('.png', ''));
    
    // Validate that avatar number is within the allowed range (1-6)
    if (isNaN(avatarNumber) || avatarNumber < 1 || avatarNumber > 6) {
      toast.error('Invalid gradient selection');
      return;
    }
    
    try {
      await updateUser({...user, avatar: avatarNumber});
      fetchAndStoreUserData();
      toast.success('Gradient updated successfully');
    } catch (error) {
      toast.error('Failed to update profile gradient');
    }
  }

  async function savePersonalInfo() {
    if (!user) return;

    // Update user store with form values
    user.name = userName;
    user.alias = userAlias;
    
    try {  
      await updateUser(user);
      fetchAndStoreUserData();
      toast.success('Profile information updated successfully');
    } catch (error) {
      toast.error('Failed to update profile information');
    }
  }

</script>

<div class="min-h-screen mt-4 px-5">
  <div class="w-full mx-auto py-8">
    <!-- Main Section -->
    <section class="mb-12 mt-4">
      <h2 class="text-xl font-medium mb-4 text-gray-900 dark:text-white">Account Settings</h2>
      
      <!-- Profile Gradient Card -->
      <div class="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6 mb-6">
        
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
          <div class="space-y-2">
            <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100">Profile Gradient</h4>
            <p class="text-sm text-gray-600 dark:text-gray-400">Choose a gradient for your profile</p>
          </div>
          
          <div class="flex items-center gap-3">
            <div class="relative">
              <button
                id="gradient-dropdown"
                class="flex items-center gap-2 px-3 py-2 text-sm font-medium bg-white dark:bg-gray-700 rounded-md shadow-sm hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors border border-gray-200 dark:border-gray-600"
                on:click={() => isGradientDropdownOpen = !isGradientDropdownOpen}
              >
                <div class="w-6 h-6 rounded-full overflow-hidden border border-gray-200 dark:border-gray-600">
                  <img
                    src={selectedGradient || `/gradients/gradient-${normalizeAvatarNumber(user?.avatar)}.png`}
                    alt="Selected gradient"
                    class="w-full h-full object-cover"
                  />
                </div>
                <span class="text-gray-800 dark:text-gray-200">Select</span>
                <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 text-gray-500 dark:text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
              </button>
              
              {#if isGradientDropdownOpen}
                <div class="absolute right-0 mt-2 w-64 bg-white dark:bg-gray-700 rounded-md shadow-lg z-10 p-3 border border-gray-200 dark:border-gray-600">
                  <div class="grid grid-cols-3 gap-2">
                    {#each gradients as gradient}
                      <button
                        class="p-1 rounded-md hover:bg-gray-100 dark:hover:bg-gray-600 transition-colors {selectedGradient === gradient ? 'ring-2 ring-blue-500' : ''}"
                        on:click={() => {
                          selectedGradient = gradient;
                          isGradientDropdownOpen = false;
                          saveProfileGradient(gradient);
                        }}
                      >
                        <img
                          src={gradient}
                          alt="Gradient option"
                          class="w-full h-16 object-cover rounded"
                        />
                      </button>
                    {/each}
                  </div>
                </div>
              {/if}
            </div>
          </div>
        </div>
      </div>

      <!-- Personal Information Card -->
      <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6 mb-6">
        <h3 class="text-lg font-medium text-gray-900 dark:text-gray-100 mb-4">Profile Information</h3>
        
        <div class="space-y-4">
          <div class="flex flex-col md:flex-row gap-4">
            <div class="flex-1">
              <label for="fullName" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Full Name
              </label>
              <input
                type="text"
                id="fullName"
                bind:value={userName}
                on:keydown={(e) => e.key === 'Enter' && savePersonalInfo()}
                class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
                placeholder="Enter your full name"
              />
            </div>
            
            <div class="flex-1">
              <label for="alias" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                What should we call you?
              </label>
              <input
                type="text"
                id="alias"
                bind:value={userAlias}
                on:keydown={(e) => e.key === 'Enter' && savePersonalInfo()}
                class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
                placeholder="What would you like to be called?"
              />
            </div>
          </div>
          
          <p class="text-sm text-gray-500 dark:text-gray-400">
            Your preferred name/alias is how you'll appear in the application.
          </p>
          
          {#if hasNameChanges}
            <div 
              class="flex justify-end overflow-hidden" 
              transition:slide={{ duration: 200 }}
            >
              <button
                class="px-4 py-2 text-sm font-medium text-white bg-primary rounded-md hover:bg-blue-600 transition-colors shadow-sm"
                on:click={savePersonalInfo}
                in:fade={{ duration: 100, delay: 50 }}
              >
                Save
              </button>
            </div>
          {/if}
        </div>
      </div>
    </section>
  </div>
</div>
