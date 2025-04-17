export const Routes = {
    chat: "/chat",
    profileAccount: "/profile/account",
    profileIntegration: "/profile/integration",
    profileSettings: "/profile/settings",
  } as const;
  
  export type ValidRoute = (typeof Routes)[keyof typeof Routes];
  
  export function isValidRoute(path: string): path is ValidRoute {
    return Object.values(Routes).includes(path.toLowerCase() as ValidRoute);
  }
  