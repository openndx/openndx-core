# Custom policy classes

How to add an access level to the Policy Decision Point without changing its code. The built-in classes and the decision format are in [POLICY_MODEL.md](POLICY_MODEL.md).

A policy class is a [Rego](https://www.openpolicyagent.org/docs/policy-language) module that decides whether an application may read one field. The PDP has four built in: `allowlist`, `allowlist_consent`, `consent` and `open`. A deployment that needs another rule, such as access limited to a period of time, writes its own class, puts the file in a directory and names that directory in the PDP's configuration.

A complete example with tests is in [examples/policies](examples/policies): the `access_window` class, which lets allow-listed applications read a field only between two dates.

## Write a class

A class is a Rego file in package `openndx.classes.<name>` with a `decision` rule. `<name>` is the value fields carry as their `policyClass`. Use letters, digits and underscores.

```rego
# Allow-listed applications read the field only inside a time window.
package openndx.classes.access_window

import data.openndx.lib

decision := lib.deny("invalid_params") if {
	not valid_params
} else := lib.deny("not_allowlisted") if {
	not lib.has_grant
} else := lib.expired if {
	lib.grant_expired
} else := lib.deny("outside_access_window") if {
	not in_window
} else := lib.allow

valid_params if {
	time.parse_rfc3339_ns(input.field.params.notBefore) <= time.parse_rfc3339_ns(input.field.params.notAfter)
}

in_window if {
	now := time.parse_rfc3339_ns(input.now)
	now >= time.parse_rfc3339_ns(input.field.params.notBefore)
	now <= time.parse_rfc3339_ns(input.field.params.notAfter)
}
```

### Input

The PDP evaluates the class once for each requested field.

| Input                 | Meaning                                                                                              |
|-----------------------|------------------------------------------------------------------------------------------------------|
| `app.id`              | The requesting application's IdP `client_id`.                                                        |
| `field.schemaId`      | The provider schema of the field.                                                                    |
| `field.fieldName`     | The field path, for example `person.fullName`.                                                       |
| `field.policyClass`   | The class name.                                                                                      |
| `field.params`        | The field's `policyParams`. Absent when the field has none.                                          |
| `grant`               | The application's active grant for the field. Absent when it has none.                               |
| `grant.expiresAt`     | RFC 3339 expiry of the grant.                                                                        |
| `grant.consentBypass` | `true` for a [consent bypass grant](POLICY_MODEL.md#consent-bypass).                                 |
| `now`                 | RFC 3339 evaluation time.                                                                            |

The PDP passes these facts in and loads nothing else: a class can't query the database or call another service. The network built-ins `http.send` and `net.lookup_ip_addr` are not available, and a module that uses one does not compile.

### Decision

`decision` is an object with an `outcome`, and optionally a `reason` and `obligations`.

| Outcome            | Meaning                                                                                      |
|--------------------|----------------------------------------------------------------------------------------------|
| `allow`            | The application may read the field.                                                          |
| `deny`             | The application is not authorized. The field is listed in `unauthorizedFields`.              |
| `expired`          | The application's access has expired. The field is listed in `expiredFields`.                |
| `consent_required` | The data owner must consent first. The field is listed in `consentRequiredFields`.           |

The set of outcomes is fixed, and the Orchestration Engine acts on the outcome alone: it never looks at the class name. `reason` is a short code saying why, such as `outside_access_window`; it appears in the decision response and the audit trail. `obligations` is a list or set of strings naming extra actions for the Orchestration Engine; see [Field outcomes](POLICY_MODEL.md#field-outcomes).

### Helpers

`data.openndx.lib` holds what the built-in classes use. Import it rather than repeating the checks.

| Helper                     | Meaning                                                                                   |
|----------------------------|-------------------------------------------------------------------------------------------|
| `lib.has_app`              | The request names an application.                                                         |
| `lib.has_grant`            | The application has a grant for the field.                                                |
| `lib.grant_expired`        | The grant's expiry has passed.                                                            |
| `lib.consent_bypassed`     | The grant is an unexpired consent bypass grant.                                           |
| `lib.allow`                | `{"outcome": "allow"}`                                                                    |
| `lib.consent_bypass_allow` | `allow` with reason `consent_bypassed` and the `audit.consent_bypassed` obligation.       |
| `lib.consent_required`     | `{"outcome": "consent_required"}`                                                         |
| `lib.expired`              | `{"outcome": "expired", "reason": "grant_expired"}`                                       |
| `lib.deny(reason)`         | `{"outcome": "deny", "reason": reason}`                                                   |

A class that asks for consent should honor consent bypass grants the way `consent` and `allowlist_consent` do: return `lib.consent_bypass_allow` when `lib.consent_bypassed` holds. The PDP accepts a bypass grant on any field except `open` and `allowlist` ones, so a custom class that ignores `grant.consentBypass` treats a bypass grant as an ordinary grant.

Helpers of your own go in a package outside `openndx`, for example `acme.lib`, in the same directory.

### A class must always answer

The PDP denies the field whenever a class gives no usable answer. Write the class so that it never comes to that, because the reasons are less helpful than your own.

| What happens                                                       | Result                                  |
|--------------------------------------------------------------------|-----------------------------------------|
| No `decision` rule matches the input                               | `deny`, reason `unknown_policy_class`   |
| A built-in function fails, such as parsing a malformed timestamp   | `deny`, reason `evaluation_error`       |
| Two `decision` rules give different answers for the same input     | `deny`, reason `evaluation_error`       |
| `outcome` is not one of the four, or `obligations` is malformed    | `deny`, reason `invalid_decision`       |

End the rule with an `else` so every input gets an answer, as the example does. The PDP does not check `policyParams` when a field is saved, so the class has to deny a field whose parameters are missing or wrong: `access_window` returns `invalid_params`.

## Test it

Write tests in a `_test.rego` file next to the class. Tests go through the router, `data.openndx.pdp.decision`, so they cover what the PDP evaluates:

```rego
package openndx.classes.access_window_test

import data.openndx.pdp

test_denies_after_window if {
	pdp.decision == {"outcome": "deny", "reason": "outside_access_window"} with input as {
		"now": "2026-12-01T00:00:00Z",
		"field": {
			"policyClass": "access_window",
			"params": {"notBefore": "2026-10-01T00:00:00Z", "notAfter": "2026-11-30T23:59:59Z"},
		},
		"grant": {"expiresAt": "2027-01-01T00:00:00Z"},
	}
}
```

Run them with the [`opa`](https://www.openpolicyagent.org/docs) command, giving it the PDP's built-in modules and your directory. From a checkout of this repository:

```bash
opa test internal/pdp/engine/policies cmd/pdp/examples/policies
```

The built-in modules are in [`internal/pdp/engine/policies`](../../internal/pdp/engine/policies). Use the ones from the PDP version you deploy. They are needed for testing only: don't copy them into your modules directory.

One difference from `opa test`: the PDP treats a failing built-in function as an error and denies the field, where `opa test` treats it as undefined. Test malformed parameters against a running PDP if your class depends on that.

## Deploy it

Set `policy.modulesDir` in the PDP's configuration to the directory that holds the class:

```yaml
policy:
  modulesDir: /app/policies
```

At startup the PDP compiles every `.rego` file under that directory, in subdirectories too, together with the built-in modules. It skips `_test.rego` files, so tests can stay next to the classes, and hidden files and directories. It logs the classes it loaded, here in the `text` log format:

```
level=INFO msg="Policy engine ready" service=policy-decision-point version=dev classes="[access_window allowlist allowlist_consent consent open]" modules_dir=/app/policies
```

With Docker Compose, mount the directory into the PDP container:

```yaml
policy-decision-point:
  volumes:
    - ./cmd/pdp/config.docker.yaml:/app/config/config.yaml:ro
    - ./my-policies:/app/policies:ro
```

On Kubernetes, mount a ConfigMap of the `.rego` files at that path.

**The PDP refuses to start** when:

- the directory does not exist or is not a directory;
- a module has a syntax error or does not compile, which includes calling `http.send` or `net.lookup_ip_addr`;
- a module redefines a built-in class;
- a module is in another package under `openndx`, such as the router `openndx.pdp` or the helpers `openndx.lib`. Only `openndx.classes.<name>` is open to custom modules;
- a class has no `decision` rule.

The error names the file. Fix the module, or remove `policy.modulesDir`, and start the PDP again.

Things to know:

- **Changes need a restart.** The PDP reads the directory once, at startup.
- **Every PDP instance needs the same directory.** An instance without a class denies its fields.
- **Removing a class denies its fields.** Fields that still name it are denied with reason `unknown_policy_class` until they get another class. Change the fields first.
- **Review modules like code.** A class decides who reads personal data. Keep the directory read-only to the PDP and under version control.

## Use it

Once the PDP has loaded the class, a field uses it like a built-in one:

- In a provider schema: `@accessControl(policy: "access_window")`.
- With `ondx`: `--field household.size:access_window:primary`.
- For a field that already exists, and to set its parameters, `PATCH /api/v1/schemas/{schemaId}/policy-metadata/{id}` on Portal Backend:

  ```json
  {"policyClass": "access_window", "policyParams": {"notBefore": "2026-10-01T00:00:00Z", "notAfter": "2026-11-30T23:59:59Z"}}
  ```

A `policyClass` the PDP has not loaded is rejected with HTTP 400, and the error lists the classes it has.
