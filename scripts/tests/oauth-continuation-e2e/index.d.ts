import type { Page, expect, test } from "@playwright/test";

export type OAuthContinuationProduct = {
  name: string;
  platformOrigin: string;
  accountCenterOrigin: string;
  productOrigin: string;
  clientID: string;
  redirectURI: string;
  productName: string;
  emailLocalPart: string;
  password: string;
  finalURL: string;
  expectedSession: Record<string, unknown>;
  publicRoutes: Array<{
    path: string;
    expectedPath?: string;
    readySelector: string;
    accountCenter?: boolean;
    isolatedSignedOut?: boolean;
  }>;
  start: (page: Page) => Promise<void>;
  restart: (page: Page) => Promise<void>;
};

export function defineOAuthContinuationJourney(
  product: OAuthContinuationProduct,
  playwright: { expect: typeof expect; test: typeof test },
): void;
