module "billing_alert" {
  source = "billtrust/billing-alarm/aws"

  aws_env = local.account_name
  # aws_account_id            = var.core_account_id
  monthly_billing_threshold = 10
}
