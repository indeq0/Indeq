export const Routes = {
    chat: "/chat",
    chatId: "/chat/:id",
    profileAccount: "/profile/account",
    profileIntegration: "/profile/integration",
    profileSettings: "/profile/settings",
  } as const;
  
  export type ValidRoute = (typeof Routes)[keyof typeof Routes];
  
  export function isValidRoute(path: string): path is ValidRoute {
    // static routes
    if (Object.values(Routes).includes(path.toLowerCase() as ValidRoute)) {
      return true;
    }
    
    // dynamic routes
    for (const route of Object.values(Routes)) {
      if (route.includes(':')) {
        const routePattern = route.replace(/:\w+/g, '[^/]+');
        const regex = new RegExp(`^${routePattern}$`);
        if (regex.test(path.toLowerCase())) {
          return true;
        }
      }
    }
    
    return false;
  }
  