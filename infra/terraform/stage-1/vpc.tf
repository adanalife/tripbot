# this is the VPC that comes pre-installed in every AWS account
module "default_vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 2.44"

  create_vpc         = false
  manage_default_vpc = true
  default_vpc_name   = "default"

  azs = data.aws_availability_zones.available.names[*]

  # cidr: 172.31.0.0/16
  public_subnets   = var.vpc_public_subnet_cidrs
  private_subnets  = var.vpc_private_subnet_cidrs
  database_subnets = var.vpc_database_subnet_cidrs

  create_database_subnet_group = true

  enable_dns_hostnames = true
  enable_dns_support   = true

  enable_nat_gateway = true
  single_nat_gateway = true

  # enable_dhcp_options      = true
  # dhcp_options_domain_name = "${local.account_name}.${var.dev_domain}"

  default_vpc_enable_dns_hostnames = true
}
