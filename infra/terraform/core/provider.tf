provider aws {
  region = var.region
}

# this lets us get the current account_id
data aws_caller_identity current {}


# set the AWS account alias
resource aws_iam_account_alias alias {
  account_alias = local.account_name
}
