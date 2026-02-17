# React + TypeScript + Vite

This template provides a minimal setup to get React working in Vite with HMR and some ESLint rules.

## API Mode

The app can run against the mock API or the engine control-plane API.

- `VITE_USE_MOCK_API=true` uses the in-memory mock API.
- `VITE_USE_MOCK_API=false` uses HTTP client mode.
- If `VITE_USE_MOCK_API` is unset, the app defaults to mock mode.
- `VITE_API_BASE_URL` sets the backend base URL in HTTP client mode.
- `VITE_ADMIN_TOKEN` sets the bearer token used by HTTP client mode.

Example:

```bash
VITE_USE_MOCK_API=false \
VITE_API_BASE_URL=http://127.0.0.1:8080 \
VITE_ADMIN_TOKEN=local-admin-token \
npm run dev
```

## Backend Mode Quickstart

1. Create local env config:
```bash
cp .env.example .env.local
```
2. Ensure backend is running with:
- `CONTROLPLANE_ADMIN_TOKENS` including the token in `VITE_ADMIN_TOKEN`
- `CONTROLPLANE_CORS_ALLOWED_ORIGINS` including `http://localhost:5173`
3. Start frontend:
```bash
npm run dev
```
4. Run smoke checks:
```bash
npm run smoke:web
```

Common failures:
- `401 unauthorized` on table fetch:
  `VITE_ADMIN_TOKEN` does not match `CONTROLPLANE_ADMIN_TOKENS`.
- CORS preflight failure in browser or smoke script:
  `CONTROLPLANE_CORS_ALLOWED_ORIGINS` is missing `http://localhost:5173`.
- Empty table list:
  Backend has no tables yet. Create one via API or `scripts/api-local.sh create-table`.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Babel](https://babeljs.io/) (or [oxc](https://oxc.rs) when used in [rolldown-vite](https://vite.dev/guide/rolldown)) for Fast Refresh
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/) for Fast Refresh

## React Compiler

The React Compiler is not enabled on this template because of its impact on dev & build performances. To add it, see [this documentation](https://react.dev/learn/react-compiler/installation).

## Expanding the ESLint configuration

If you are developing a production application, we recommend updating the configuration to enable type-aware lint rules:

```js
export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      // Other configs...

      // Remove tseslint.configs.recommended and replace with this
      tseslint.configs.recommendedTypeChecked,
      // Alternatively, use this for stricter rules
      tseslint.configs.strictTypeChecked,
      // Optionally, add this for stylistic rules
      tseslint.configs.stylisticTypeChecked,

      // Other configs...
    ],
    languageOptions: {
      parserOptions: {
        project: ['./tsconfig.node.json', './tsconfig.app.json'],
        tsconfigRootDir: import.meta.dirname,
      },
      // other options...
    },
  },
])
```

You can also install [eslint-plugin-react-x](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-x) and [eslint-plugin-react-dom](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-dom) for React-specific lint rules:

```js
// eslint.config.js
import reactX from 'eslint-plugin-react-x'
import reactDom from 'eslint-plugin-react-dom'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      // Other configs...
      // Enable lint rules for React
      reactX.configs['recommended-typescript'],
      // Enable lint rules for React DOM
      reactDom.configs.recommended,
    ],
    languageOptions: {
      parserOptions: {
        project: ['./tsconfig.node.json', './tsconfig.app.json'],
        tsconfigRootDir: import.meta.dirname,
      },
      // other options...
    },
  },
])
```
