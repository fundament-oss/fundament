import { AuthnService } from '../generated/authn/v1/authn_pb';
import { OrganizationService } from '../generated/v1/organization_pb';
import { ClusterService } from '../generated/v1/cluster_pb';
import { createClientToken, AUTHN_TRANSPORT, ORGANIZATION_TRANSPORT } from './connect.module';

// Create an injection token for the Authn service client
export const AUTHN = createClientToken(AuthnService, AUTHN_TRANSPORT);

// Create an injection token for the Organization service client
export const ORGANIZATION = createClientToken(OrganizationService, ORGANIZATION_TRANSPORT);

// Create an injection token for the Cluster service client
export const CLUSTER = createClientToken(ClusterService, ORGANIZATION_TRANSPORT);
