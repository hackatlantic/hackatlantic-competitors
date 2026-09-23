variable "api_wallet_env" {
  description = "Optional server-only Google Wallet settings from the protected environment secret."
  type        = map(string)
  sensitive   = true
  default     = {}

  validation {
    condition = alltrue([
      for key in keys(var.api_wallet_env) : contains([
        "GOOGLE_WALLET_ENABLED",
        "GOOGLE_WALLET_ISSUER_ID",
        "GOOGLE_WALLET_CLASS_ID",
        "GOOGLE_WALLET_CYCLE_SLUG",
        "GOOGLE_WALLET_SERVICE_ACCOUNT_JSON",
      ], key)
    ])
    error_message = "api_wallet_env may only set Google Wallet variables, never other API settings."
  }
}
