# ThunderID configuration

[ThunderID](https://github.com/thunder-id/thunderid) is the identity provider of the local stack. It issues the tokens that the Orchestration Engine, the Consent Engine and Portal Backend check, and Portal Backend creates each application's OAuth2 client in it. [compose.yml](../compose.yml) runs ThunderID 1.1.1 with the files in this folder, and the [Helm chart](../deploy/helm/openndx/README.md#embedded-identity-provider-thunderid) loads the same bootstrap files.

## Files

| File                                                     | What it sets up                                                                                                                                                                         |
| -------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [deployment.yaml](deployment.yaml)                       | ThunderID's server settings. `server.public_url` is the issuer (`iss`) of every token, so it must equal `IDP_PUBLIC_URL`, the issuer the services check.                                |
| [bootstrap/users.yaml](bootstrap/users.yaml)             | The `Citizen` user type and the sample citizens, the `ndx-admin` operator, the sample organization units with a staff user each, and the `OpenNDX Admins` and `OpenNDX Members` groups. |
| [bootstrap/resource.yaml](bootstrap/resource.yaml)       | The resource servers of the Orchestration Engine (`http://api.openndx.local`) and Portal Backend (`http://pb.openndx.local`) with their scopes, and the roles that grant them.          |
| [bootstrap/application.yaml](bootstrap/application.yaml) | The OAuth2 clients `CONSENT_PORTAL`, `PORTAL_BACKEND` and `NDX_CLI`, and the sample `DATA_CONSUMER`.                                                                                    |
| [bootstrap/cors.yaml](bootstrap/cors.yaml)               | The browser origins ThunderID accepts: the Consent Portal's and that of the `ondx login` callback.                                                                                      |

A `{{ .NAME }}` value in a bootstrap file, such as `{{ .PORTAL_BACKEND_CLIENT_SECRET }}`, is read from the environment of the `thunderid-setup` service.

## How Compose applies them

`thunderid-db-init` copies the image's databases into the `thunderid-db` volume when the volume is empty. `thunderid-setup` then runs ThunderID's `setup.sh`, which generates the signing keys and the Direct API secret if they don't exist yet and imports the bootstrap files. Then `thunderid` serves on port 8090.

`thunderid-setup` runs on every `docker compose up`, and the import replaces each resource with the version in the files, so an edit to a bootstrap file takes effect on the next `docker compose up`. What was created at runtime, such as the clients Portal Backend creates for applications, stays in the volume until `docker compose down -v` deletes it.

## Entries for local use only

These exist for the [walkthrough](../dev/WALKTHROUGH.md) and the [sample data](../dev/README.md). For a shared deployment, remove them or replace their passwords and secrets:

- in `users.yaml`, the `Citizen` user type and the sample citizens, the `ndx-admin` operator, and the sample organization units and their staff users, all with password `1234`. Keep the `OpenNDX Admins` and `OpenNDX Members` groups: the roles in `resource.yaml` are assigned to them;
- the `DATA_CONSUMER` client in `application.yaml`, and its assignment to the `DataConsumerGetData` role in `resource.yaml`. Keep the role: Portal Backend assigns it to every application it creates;
- the `NDX_CLI` client, which the `local` profile of the `ondx` CLI logs in with. For another deployment, register a client for `ondx` and save it in a profile ([cmd/ondx/README.md](../cmd/ondx/README.md#configuration));
- the local origins in `cors.yaml`.

Sign-up in the Consent Portal is off: the citizens are the accounts in `users.yaml`. An account that signs itself up is a `Person`, which has no `nic`, the claim the Consent Engine reads a data owner's ID from (`IDP_SUBJECT_CLAIM`), so it couldn't use the Consent Portal.

Signed-in users can still change their own attributes, `nic` included, with `PUT /users/me`, which ThunderID keeps open to every signed-in user. The local stack leaves it open, since its sample passwords are public; a shared deployment must refuse it ([Helm README](../deploy/helm/openndx/README.md#refuse-self-service-attribute-changes)).
