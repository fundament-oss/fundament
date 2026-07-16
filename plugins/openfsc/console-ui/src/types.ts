// The subset of the FSCInstallation CR (openfsc.fundament.io/v1) the console
// views read. Deliberately partial/optional — the SDK returns raw cluster JSON.

export interface GatewayStatus {
  name?: string;
  phase?: string;
  url?: string;
  message?: string;
}

export interface Condition {
  type?: string;
  status?: string;
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}

export interface FSCInstallation {
  metadata?: {
    name?: string;
    namespace?: string;
    creationTimestamp?: string;
  };
  spec?: {
    groupID?: string;
    peerID?: string;
    directory?: {
      mode?: string;
      external?: { address?: string; peerID?: string };
    };
  };
  status?: {
    phase?: string;
    message?: string;
    managerAddress?: string;
    controllerURL?: string;
    conditions?: Condition[];
    inways?: GatewayStatus[];
    outways?: GatewayStatus[];
  };
}
