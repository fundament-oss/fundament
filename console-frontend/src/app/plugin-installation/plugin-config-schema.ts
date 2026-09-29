import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import type { ObservableClient } from '../../connect/observable-client';
import type { InstallService } from '../../generated/install/v1/install_pb';
import {
  GetPluginDefinitionRequestSchema,
  type ConfigSchemaEntry,
} from '../../generated/catalog/v1/catalog_pb';

/**
 * Fetches the config schema a published version declares, through install.v1
 * (the organization's read surface post-FUN-22). Shared by the install modal
 * and the cluster bulk form so the request is spelled once.
 *
 * Throws when the catalog reports the schema unavailable: the caller cannot
 * tell whether required config exists, so it must fail closed exactly like a
 * fetch error — installing past an unseen required key creates a CR the
 * controller terminally fails.
 */
export default async function fetchPluginConfigSchema(
  client: ObservableClient<typeof InstallService>,
  organizationName: string,
  pluginName: string,
  version: string,
): Promise<ConfigSchemaEntry[]> {
  const resp = await firstValueFrom(
    client.getPluginDefinition(
      create(GetPluginDefinitionRequestSchema, {
        lookup: {
          case: 'name',
          value: { organizationName, pluginName },
        },
        version,
      }),
    ),
  );
  if (resp.configSchemaUnavailable) {
    throw new Error('config schema unavailable');
  }
  return resp.configSchema;
}
