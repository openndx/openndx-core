export const PortalAction = {
    approve: "approve",
    reject: "reject",
    revoke: "revoke",
} as const;

export type PortalAction = (typeof PortalAction)[keyof typeof PortalAction];