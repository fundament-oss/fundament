import { AuthnService } from '../generated/authn/v1/authn_pb';
import { OrganizationService } from '../generated/v1/organization_pb';
import { ClusterService } from '../generated/v1/cluster_pb';
import { PluginService } from '../generated/v1/plugin_pb';
import { ProjectService } from '../generated/v1/project_pb';
import { MemberService } from '../generated/v1/member_pb';
import { APIKeyService } from '../generated/v1/apikey_pb';
import { createClientToken, AUTHN_TRANSPORT, ORGANIZATION_TRANSPORT } from './connect.module';

// Create an injection token for the Authn service client
export const AUTHN = createClientToken(AuthnService, AUTHN_TRANSPORT);

// Create an injection token for the Organization service client
export const ORGANIZATION = createClientToken(OrganizationService, ORGANIZATION_TRANSPORT);

// Create an injection token for the Cluster service client
export const CLUSTER = createClientToken(ClusterService, ORGANIZATION_TRANSPORT);

// Create an injection token for the Plugin service client
export const PLUGIN = createClientToken(PluginService, ORGANIZATION_TRANSPORT);

// Create an injection token for the Project service client
export const PROJECT = createClientToken(ProjectService, ORGANIZATION_TRANSPORT);

// Create an injection token for the Member service client
export const MEMBER = createClientToken(MemberService, ORGANIZATION_TRANSPORT);

// Create an injection token for the API Key service client
export const APIKEY = createClientToken(APIKeyService, ORGANIZATION_TRANSPORT);
