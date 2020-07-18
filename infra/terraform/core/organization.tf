# create the parent AWS Organization (adanalife-core is the root)
resource aws_organizations_organization org {
  feature_set = "ALL"
}

# this will loop over all other accounts in this repo
# and create an AWS account that is attached to this organization
resource aws_organizations_account account {
  count     = length(local.account_names)
  name      = local.account_names[count.index]
  email     = "${var.email_prefix}${local.account_names[count.index]}@${var.email_domain}"
  role_name = var.admin_role

  lifecycle {
    ignore_changes = [role_name]
  }

  depends_on = [aws_organizations_organization.org]
}
