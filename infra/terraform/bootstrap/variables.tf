variable state_bucket {
  type = string
}

variable admin_group_name {
  type    = string
  default = "Admin"
}

variable dynamodb_table {
  type    = string
  default = "terraform-state-lock"
}

variable region {
  type    = string
  default = "us-east-1"
}

variable admin_users {
  type    = list(string)
  default = []
}

locals {
  create_iam = length(var.admin_users) > 0
}
