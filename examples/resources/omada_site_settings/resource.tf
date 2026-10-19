# One settings resource owns rotations for this site.
# Review all planned defaults when adopting or importing existing settings.
# Destroying this resource forgets it without changing the controller.
resource "omada_site_settings" "example" {
  site_id = omada_site.example.id

  led_enable          = true
  mesh_enable         = true
  fast_roaming_enable = true
  lldp_enable         = true

  band_steering_enable               = true
  band_steering_connection_threshold = 30
  band_steering_difference_threshold = 4
  band_steering_max_failures         = 5

  device_account_username = "admin"
  device_account_password = var.device_password
}

variable "device_password" {
  type      = string
  sensitive = true
}
