# create a group for developer users
resource aws_iam_group developer {
  name = var.developer_group
}

# attach the assume_role policy to the Developer group
resource aws_iam_group_policy_attachment developer_assume_role {
  group      = var.developer_group
  policy_arn = aws_iam_policy.developer_assume_role.arn

  depends_on = [aws_organizations_account.account]
}

resource aws_iam_group_policy_attachment developer_self_management {
  group      = var.developer_group
  policy_arn = aws_iam_policy.self_management.arn
}

# attach the assume_role policy to the Admin group
resource aws_iam_group_policy_attachment admin_assume_role {
  group      = var.admin_group
  policy_arn = aws_iam_policy.admin_assume_role.arn

  depends_on = [aws_organizations_account.account]
}
