# ES216G Terraform acceptance: operator instructions

These instructions record the physical switch setup completed for the Terraform acceptance run. The [acceptance plan](../plans/es216g-terraform-acceptance.md) tracks remaining implementation and validation work.

## Controller and switch addresses

- Omada Software Controller: `https://192.168.4.24:8043`
- ES216G management address: `https://192.168.4.22`
- Switch MAC: `D4-D6-DF-1C-3B-58`
- Switch: ES216G hardware revision 1.20, firmware `1.0.1 Build 20260329 Rel.14489`

The controller uses a local self-signed certificate. Chrome may show a certificate warning when opening the controller console; use the local controller certificate exception/trust already established for this lab.

## Successful adoption

1. In the switch standalone UI, open **System → Controller Settings**.
2. Leave **Cloud-Based Controller Management** disabled and **Remote Adoption** enabled.
3. Set **Inform URL/IP Address** to `192.168.4.24`, then click **Apply**.
4. In the software controller, open **Devices → Add Devices** and select the ES216G.
5. If automatic adoption fails because the device credentials differ, retry with the username and password currently used to sign in to the switch at `https://192.168.4.22`.
6. Confirm the adoption result says **CONNECTED** and the MAC is `D4-D6-DF-1C-3B-58`.

![Omada controller reports successful ES216G adoption](images/es216g-controller-adopted.png)

### Credential distinction

`.es216g-lab/credentials.json` contains the controller administrator login and the controller site's generated device-management account. Those values are not necessarily the switch's current standalone login. The first adoption attempt using the generated device-account credentials failed; using the switch's current standalone credentials succeeded. Never put any of these passwords in documentation, Terraform files, or chat.

The CSV imported into Chrome Password Manager contains only the local controller administrator login. It does not contain the switch's standalone credentials.

## Current state and next steps

The ES216G is adopted and connected to the physical-test controller. No port settings have been deliberately edited for the Terraform test; adoption side effects have not yet been reconciled against the pre-adoption inventory. No configuration backup was made, per the user's instruction.

Before running Terraform writes:

1. Read the port inventory in the controller and compare it with the switch's observed standalone inventory.
2. Identify the uplink and a link-down, unused non-uplink test port; do not guess from port number.
3. Perform the Terraform import and omitted-`ports` stable-plan check from the acceptance plan.
4. Only after those checks pass, perform the planned reversible display-name update on the verified unused port.
