export const isIntegrated = (integrations: string[], provider: string): boolean => {
    return integrations.includes(provider.toUpperCase());
};