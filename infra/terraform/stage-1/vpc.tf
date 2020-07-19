# this is the VPC that comes pre-installed in every AWS account
module "default_vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 2.44"

  create_vpc         = false
  manage_default_vpc = true
  default_vpc_name   = "default"

  default_vpc_enable_dns_hostnames = true
}
