import { vitePreprocess } from '@sveltejs/vite-plugin-svelte'

export default {
  preprocess: vitePreprocess(),
  compilerOptions: {
    runes: true, // только Svelte 5 runes, никакого legacy-синтаксиса
  },
}
