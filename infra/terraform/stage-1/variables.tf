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

variable vpc_public_subnet_cidrs {
  type        = list(string)
  description = "A list of CIDRs for the public subnet"
}

variable vpc_private_subnet_cidrs {
  type        = list(string)
  description = "A list of CIDRs for the private subnet"
}

variable vpc_database_subnet_cidrs {
  type        = list(string)
  description = "A list of CIDRs for database subnets"
}

variable vpc_dns_servers {
  type        = list(string)
  description = "A list of DNS server IPs"
}

locals {
  org_name = "adanalife"
  # this is how we will refer to the account in other places
  account_name      = "${var.environment}-${var.label}"
  full_account_name = "${local.org_name}-${var.environment}-${var.label}"
}
