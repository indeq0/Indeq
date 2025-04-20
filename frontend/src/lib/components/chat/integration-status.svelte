<script lang="ts">
    import { isIntegrated } from "$lib/utils/integration";
    export let integrations: string[] = [];
    export let desktopIntegration;
</script>
  
<div class="flex gap-2">
    <!-- Desktop Integration -->
    <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
      <div class="relative">
        <div
          class="w-2 h-2 rounded-full"
          style="background-color: {desktopIntegration && desktopIntegration.isCrawling && desktopIntegration.crawledFiles != desktopIntegration.totalFiles ? 'orange' : desktopIntegration && desktopIntegration.isOnline ? 'green' : 'red'}" 
        ></div>
        <div
          class="w-2 h-2 rounded-full absolute top-0 animate-ping"
          style="background-color: {desktopIntegration && desktopIntegration.isCrawling && desktopIntegration.crawledFiles != desktopIntegration.totalFiles ? 'orange' : desktopIntegration && desktopIntegration.isOnline ? 'green' : 'red'}"
        ></div>
      </div>
      <span class="text-xs text-gray-600 ml-1">Desktop {desktopIntegration && desktopIntegration.isCrawling && desktopIntegration.crawledFiles != desktopIntegration.totalFiles ? desktopIntegration.crawledFiles + ' / ' + desktopIntegration.totalFiles + ' files' : ""}</span>
    </div>
    
    <!-- Google -->
    <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
      <div class="relative">
        <div
          class="w-2 h-2 rounded-full"
          style="background-color: {isIntegrated(integrations, 'GOOGLE') ? 'green' : 'red'}"
        ></div>
        <div
          class="w-2 h-2 rounded-full absolute top-0 animate-ping"
          style="background-color: {isIntegrated(integrations, 'GOOGLE') ? 'green' : 'red'}"
        ></div>
      </div>
      <span class="text-xs text-gray-600 ml-1">Google</span>
    </div>
    
    <!-- Microsoft -->
    <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
      <div class="relative">
        <div
          class="w-2 h-2 rounded-full"
          style="background-color: {isIntegrated(integrations, 'MICROSOFT') ? 'green' : 'red'}"
        ></div>
        <div
          class="w-2 h-2 rounded-full absolute top-0 animate-ping"
          style="background-color: {isIntegrated(integrations, 'MICROSOFT') ? 'green' : 'red'}"
        ></div>
      </div>
      <span class="text-xs text-gray-600 ml-1">Microsoft</span>
    </div>
    
    <!-- Notion -->
    <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
      <div class="relative">
        <div
          class="w-2 h-2 rounded-full"
          style="background-color: {isIntegrated(integrations, 'NOTION') ? 'green' : 'red'}"
        ></div>
        <div
          class="w-2 h-2 rounded-full absolute top-0 animate-ping"
          style="background-color: {isIntegrated(integrations, 'NOTION') ? 'green' : 'red'}"
        ></div>
      </div>
      <span class="text-xs text-gray-600 ml-1">Notion</span>
    </div>
</div>