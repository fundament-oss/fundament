# outway

OpenFSC is an open source peer-to-peer system facilitating federated authentication, secure connecting and protocolling in a large-scale, dynamic API ecosystem with many organizations.
Through an Outway an organization can query services on the OpenFSC ecosystem. It's usually deployed centrally within the organization although it is possible for one organization to deploy multiple instances on different locations.

## Prerequisites

-   Kubernetes 1.11+

## Installing the Chart

To install the Chart with the release name `outway`:

```console
$ helm install outway oci://registry-1.docker.io/federatedserviceconnectivity/open-fsc-outway --version {latest version}
```

> **Tip**: List all releases using `helm list`

## Upgrading the Chart

Currently, our Helm charts use the same release version as the OpenFSC release version.
To know what has changed for the Helm charts, look at the changes in our [CHANGELOG](https://gitlab.com/rinis-oss/fsc/open-fsc/-/blob/main/CHANGELOG.md)
that are prefixed with 'Helm'.

## Uninstalling the Chart

To uninstall or delete the `outway` deployment:

```console
$ helm delete outway
```

## Parameters

### Global parameters

| Name                                                               | Description                                                                                                               | Value     |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------- | --------- |
| `global.imageRegistry`                                             | Global Docker Image registry                                                                                              | `""`      |
| `global.imageTag`                                                  | Global Docker Image tag                                                                                                   | `""`      |
| `global.groupID`                                                   | Global FSC Group ID                                                                                                       | `""`      |
| `global.imagePullSecrets`                                          | Global image pull secrets                                                                                                 | `[]`      |
| `global.certificates.group.caCertificatePEM`                       | OpenFSC CA root certificate. If not set the value of 'certificates.group.caCertificatePEM' is used                        | `""`      |
| `global.certificates.group.caCertificatePEMExistingSecret.name`    | Name of the existing secret                                                                                               | `""`      |
| `global.certificates.group.caCertificatePEMExistingSecret.key`     | The key in the secret that contains the certificate                                                                       | `tls.crt` |
| `global.certificates.internal.caCertificatePEM`                    | Global CA root certificate of your internal PKI. If not set the value of 'certificates.internal.caCertificatePEM' is used | `""`      |
| `global.certificates.internal.caCertificatePEMExistingSecret.name` | Name of the existing secret                                                                                               | `""`      |
| `global.certificates.internal.caCertificatePEMExistingSecret.key`  | The key in the secret that contains the certificate                                                                       | `tls.crt` |

### Deployment Parameter

| Name                                       | Description                                                                                                            | Value                                 |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| `image.registry`                           | Image registry (ignored if 'global.imageRegistry' is set)                                                              | `docker.io`                           |
| `image.repository`                         | Image repository of the Outway.                                                                                        | `federatedserviceconnectivity/outway` |
| `image.tag`                                | Image tag (ignored if 'global.imageTag' is set). When set to null, the AppVersion from the chart is used               | `""`                                  |
| `image.pullPolicy`                         | Image pull policy                                                                                                      | `Always`                              |
| `image.pullSecrets`                        | Image pull secrets                                                                                                     | `[]`                                  |
| `replicaCount`                             | Number of controller replicas                                                                                          | `1`                                   |
| `securityContext.allowPrivilegeEscalation` | Controls whether a process can gain more privileges than its parent process                                            | `false`                               |
| `securityContext.runAsNonRoot`             | Run container as a non-root user                                                                                       | `true`                                |
| `securityContext.runAsUser`                | Run container as specified user                                                                                        | `1001`                                |
| `securityContext.capabilities.drop`        | Drop all capabilities by default                                                                                       | `["ALL"]`                             |
| `deploymentAnnotations`                    | Annotations to add to the deployment                                                                                   | `{}`                                  |
| `podSecurityContext.fsGroup`               | GroupID under which the pod should be started                                                                          | `1001`                                |
| `resources`                                | Pod resource requests & limits                                                                                         | `{}`                                  |
| `nodeSelector`                             | Node labels for pod assignment                                                                                         | `{}`                                  |
| `affinity`                                 | Node affinity for pod assignment                                                                                       | `{}`                                  |
| `tolerations`                              | Node tolerations for pod assignment                                                                                    | `[]`                                  |
| `extraEnv`                                 | Extra env items for pod assignment                                                                                     | `[]`                                  |
| `serviceAccount.create`                    | Specifies whether a service account should be created                                                                  | `true`                                |
| `serviceAccount.annotations`               | Annotations to add to the service account                                                                              | `{}`                                  |
| `serviceAccount.name`                      | The name of the service account to use. If not set and create is true, a name is generated using the fullname template | `""`                                  |
| `podLabels`                                | Extra labels added to the pod                                                                                          | `{}`                                  |

### Common Parameters

| Name               | Description                   | Value |
| ------------------ | ----------------------------- | ----- |
| `nameOverride`     | Override deployment name      | `""`  |
| `fullnameOverride` | Override full deployment name | `""`  |

### OpenFSC Outway parameters

| Name                                                              | Description                                                                                                                                                                                                 | Value    |
| ----------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `config.logType`                                                  | Possible values 'live', 'local'. Affects the log output. See NewProduction and NewDevelopment at https://godoc.org/go.uber.org/zap#Logger                                                                   | `live`   |
| `config.logLevel`                                                 | Possible values 'debug', 'warn', 'info'. Override the default logLevel set by 'config.logType'                                                                                                              | `info`   |
| `config.enableGrantHashSuggestion`                                | Possible values 'true' and 'false'. Whether the Outway should return valid services and Grant Hashes                                                                                                        | `false`  |
| `config.groupID`                                                  | FSC Group ID                                                                                                                                                                                                | `""`     |
| `config.name`                                                     | Name of the Outway                                                                                                                                                                                          | `""`     |
| `config.managerInternalAddress`                                   | Internal address of the Manager                                                                                                                                                                             | `""`     |
| `config.controllerRegistrationApiAddress`                         | The address of the Controller API                                                                                                                                                                           | `""`     |
| `config.transactionLogApiAddress`                                 | The Address of the Transaction Log API                                                                                                                                                                      | `""`     |
| `config.authorizationService.enabled`                             | If 'true', the Outway will use the authorization service                                                                                                                                                    | `false`  |
| `config.authorizationService.url`                                 | URL of the authorization service to use                                                                                                                                                                     | `""`     |
| `config.authorizationService.caCertificatePEMExistingSecret.name` | Name of an existing secret that contains the CA certificate                                                                                                                                                 | `""`     |
| `config.authorizationService.caCertificatePEMExistingSecret.key`  | Key in the existing secret that contains the CA certificate                                                                                                                                                 | `ca.crt` |
| `config.authorizationService.withBody`                            | When set to true, the HTTP request body (if available) will be send to the Authorization Server in base64 encoded format.                                                                                   | `false`  |
| `config.authorizationService.maxBodySize`                         | The maximum HTTP request body size in bytes that is allowed for sending to the authorization server. If a body exceeds this limits, the body is not send to the Authorization Server.                       | `4096`   |
| `config.authorizationService.bodyChunkSize`                       | The chunk size in bytes that is used to process each HTTP request body chunk.                                                                                                                               | `1024`   |
| `config.authZenService.enabled`                                   | If 'true', the Inway will use the AuthZen service                                                                                                                                                           | `false`  |
| `config.authZenService.url`                                       | URL of the AuthZen service to use                                                                                                                                                                           | `""`     |
| `config.authZenService.caCertificatePEMExistingSecret.name`       | Name of an existing secret that contains the CA certificate                                                                                                                                                 | `""`     |
| `config.authZenService.caCertificatePEMExistingSecret.key`        | Key in the existing secret that contains the CA certificate                                                                                                                                                 | `ca.crt` |
| `config.authZenService.withBody`                                  | When set to true, the HTTP request body (if available) will be send to the AuthZen Server in base64 encoded format.                                                                                         | `false`  |
| `config.authZenService.maxBodySize`                               | The maximum HTTP request body size in bytes that is allowed for sending to the AuthZen server. If a body exceeds this limits, the body is not send to the Authorization Server.                             | `4096`   |
| `config.authZenService.bodyChunkSize`                             | The chunk size in bytes that is used to process each HTTP request body chunk.                                                                                                                               | `1024`   |
| `config.disableCrlChecks`                                         | If 'true', the Outway will not check if the Client Certificate of the Inway or Manager is on the Certificate Revocation List. This means the Outway will accept client certificates that have been revoked. | `false`  |
| `config.grantLinksCacheTTL`                                       | The time to live of the Grant Links cache, format is specified in string, e.g. '1h', '300s' or '5m'                                                                                                         | `30s`    |
| `config.xffAllowedAddresses`                                      | List of addresses allowed in the 'x-forwarded-for' header                                                                                                                                                   | `[]`     |

### TLS certificates used by OpenFSC components for communications

| Name                                                        | Description                                                                                                                                                                  | Value |
| ----------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| `certificates.group.caCertificatePEM`                       | The CA certificate of the Group                                                                                                                                              | `""`  |
| `certificates.group.caCertificatePEMExistingSecret.name`    | Name of the existing secret                                                                                                                                                  | `""`  |
| `certificates.group.caCertificatePEMExistingSecret.key`     | The key in the secret that contains the certificates                                                                                                                         | `""`  |
| `certificates.group.certificatePEM`                         | The Group certificate                                                                                                                                                        | `""`  |
| `certificates.group.keyPEM`                                 | Private Key of 'certificates.group.certificatePEM'                                                                                                                           | `""`  |
| `certificates.group.existingSecret`                         | Use existing secret with your OpenFSC keypair (`certificates.group.certificatePEM` and `certificates.group.keyPEM` will be ignored and picked up from the secret)            | `""`  |
| `certificates.internal.caCertificatePEM`                    | The CA root certificate of your internal PKI                                                                                                                                 | `""`  |
| `certificates.internal.caCertificatePEMExistingSecret.name` | Name of the existing secret                                                                                                                                                  | `""`  |
| `certificates.internal.caCertificatePEMExistingSecret.key`  | The key in the secret that contains the certificate                                                                                                                          | `""`  |
| `certificates.internal.certificatePEM`                      | The certificate signed by your internal PKI                                                                                                                                  | `""`  |
| `certificates.internal.keyPEM`                              | the private key of 'certificates.internal.certificatePEM'                                                                                                                    | `""`  |
| `certificates.internal.existingSecret`                      | Use of existing secret with your OpenFSC keypair ('certificates.internal.certificatePEM' and 'certificates.internal.keyPEM'. will be ingored and picked up from this secret) | `""`  |

### Exposure parameters

| Name                                  | Description                                                                             | Value       |
| ------------------------------------- | --------------------------------------------------------------------------------------- | ----------- |
| `service.type`                        | Service Type (ClusterIP, NodePort, LoadBalancer)                                        | `ClusterIP` |
| `service.annotations`                 | Annotations to be added to the service                                                  | `{}`        |
| `service.httpPort`                    | Port exposed by the service                                                             | `80`        |
| `service.httpsPort`                   | Port exposed by the service if 'https.enabled' is 'true'                                | `443`       |
| `https.enabled`                       | If 'true' HTTPs will be enabled                                                         | `false`     |
| `https.keyPEM`                        | Private Key of the 'https.certificatePEM' as PEM. Required if 'https.enabled' is 'true' | `""`        |
| `https.certificatePEM`                | TLS Certificate of PEM. Required if 'https.enabled' is 'true'                           | `""`        |
| `https.existingSecret.name`           | The name of the secret that contains the certificate and private key                    | `""`        |
| `https.existingSecret.keyCertificate` | The key in the secret that contains the certificate                                     | `tls.crt`   |
| `https.existingSecret.keyPrivateKey`  | The key in the secret that contains the private key                                     | `tls.key`   |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`.

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart.

```console
$ helm install outway -f values.yaml .
```

> **Tip**: You can use the default [values.yaml](https://gitlab.com/rinis-oss/fsc/open-fsc/blob/main/helm/charts/open-fsc-outway/values.yaml)
