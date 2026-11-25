/**
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Lanka Software Foundation
 */

export const PortalAction = {
    approve: "approve",
    reject: "reject",
    revoke: "revoke",
} as const;

export type PortalAction = (typeof PortalAction)[keyof typeof PortalAction];