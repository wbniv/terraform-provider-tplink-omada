# Bootstrap credentials are required for creation only.
# For later rotation, remove both inputs here and manage them in omada_site_settings.
resource "omada_site" "example" {
  name      = "Branch-Office"
  region    = "United States"
  time_zone = "UTC"
  scenario  = "Office"
  type      = 0

  device_account_username = "admin"
  device_account_password = var.bootstrap_device_password
}

variable "bootstrap_device_password" {
  type      = string
  sensitive = true
}
