/**
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Lanka Software Foundation
 */

export const ConsentStatus = {
    pending: 'pending',
    approved: 'approved',
    rejected: 'rejected',
    expired: 'expired',
    revoked: 'revoked',
} as const;

export type ConsentStatus = (typeof ConsentStatus)[keyof typeof ConsentStatus];
