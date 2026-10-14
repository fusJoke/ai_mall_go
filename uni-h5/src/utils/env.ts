/** `import.meta.env` 的类型安全访问。 */

export const viteEnv = {
  BASE_URL: (import.meta.env.VITE_API_BASE_URL as string) || '/api',
  USE_MOCK: (import.meta.env.VITE_USE_MOCK as string) !== 'false',
  DEV_TOKEN: (import.meta.env.VITE_DEV_TOKEN as string) || '',
  DEV_USER: (import.meta.env.VITE_DEV_USER as string) || '',
} as const
