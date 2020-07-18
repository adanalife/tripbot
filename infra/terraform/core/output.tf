# to see these values at any time, run:
#   $ terraform output

output accounts {
  value = merge(
    zipmap(aws_organizations_account.account.*.name, aws_organizations_account.account.*.id),
    { "${local.account_name}" = local.core_account_id }
  )
}
