# prod, stage, dev
variable environment {
  type = string
}

variable label {
  type        = string
  description = "An identifier for this particular environment"
  default     = "1"
}

variable region {
  type    = string
  default = "us-east-1"
}

variable core_account_id {
  type        = string
  description = "The AWS account ID for the core account"
}

variable rds_tripbot_username {
  type = string
}

variable rds_tripbot_password {
  type = string
}

locals {
  org_name = "adanalife"
  # this is how we will refer to the account in other places
  account_name      = "${var.environment}-${var.label}"
  full_account_name = "${local.org_name}-${var.environment}-${var.label}"
}
