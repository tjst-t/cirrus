# S046-1 Autonomous Decisions

## 1. localStorage key alignment (auth.ts / client.ts)

**Problem**: The Playwright test's `authInit` sets `cirrus_token` and `cirrus_tenant_id` in localStorage, but `src/lib/auth.ts` defined `TOKEN_KEY = 'auth_token'` and `TENANT_ID_KEY = 'selected_tenant_id'`. This caused `isAuthenticated()` to return false, redirecting tests to `/login`.

**Decision**: Updated `TOKEN_KEY` and `TENANT_ID_KEY` in `src/lib/auth.ts` to `cirrus_token` / `cirrus_tenant_id` to match the test spec. Updated `src/api/client.ts` to reference these constants via import instead of hardcoded strings.

**Rationale**: The test spec defines the contract; the application should conform to it.

## 2. Separate create-org error state

**Problem**: The original `OrganizationsPage` used a single `error` state for both list load errors and org create errors. The test `create-org-error` expects an error element _inside_ the dialog, separate from the `org-list-error` outside.

**Decision**: Split into `listError` (shown in `org-list-error`) and `createError` (shown in `create-org-error` inside the dialog).

## 3. Dialog / ConfirmDialog data-testid props

**Problem**: `Dialog` and `ConfirmDialog` had no `data-testid` prop support.

**Decision**: Added `data-testid` to `DialogProps` and passed it to the inner content `<div>`. Added `data-testid` and `data-testid-confirm` to `ConfirmDialogProps` for the dialog wrapper and confirm button respectively.

## 4. ErrorMessage data-testid prop

**Decision**: Added `data-testid` prop to `ErrorMessage` component so callers can attach test IDs selectively.
