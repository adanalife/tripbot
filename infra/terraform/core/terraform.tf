terraform {
  required_version = "~> 0.12"
  required_providers {
    # we don't strictly require v2.65, and should
    # probably move to v3.0 when it gets released
    # c.p. terraform.io/docs/providers/aws/index.html
    aws = "~> 2.65"
  }

  backend "s3" {
    bucket = "adanalife-core-tf-state"
    key    = "adanalife-core.tfstate"
  }
}