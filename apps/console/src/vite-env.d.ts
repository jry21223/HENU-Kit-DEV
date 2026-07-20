/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_QUIZCRAFT_WORKSHOP_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
