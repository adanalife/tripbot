resource aws_iam_policy self_management {
  name   = "AllowUsersToManageTheirOwnAccounts"
  policy = <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "AllowUserToSeeAndManageTheirOwnAccountInformation",
            "Effect": "Allow",
            "Action": [
                "iam:ChangePassword",
                "iam:CreateAccessKey",
                "iam:CreateLoginProfile",
                "iam:DeleteAccessKey",
                "iam:DeleteLoginProfile",
                "iam:DeleteSSHPublicKey",
                "iam:DeleteSigningCertificate",
                "iam:GetLoginProfile",
                "iam:GetSSHPublicKey",
                "iam:GetUser",
                "iam:ListAccessKeys",
                "iam:ListSSHPublicKeys",
                "iam:ListSigningCertificates",
                "iam:UpdateAccessKey",
                "iam:UpdateLoginProfile",
                "iam:UpdateSSHPublicKey",
                "iam:UpdateSigningCertificate",
                "iam:UploadSSHPublicKey",
                "iam:UploadSigningCertificate"
            ],
            "Resource": "arn:aws:iam::${local.core_account_id}:user/$${aws:username}"
        },
        {
            "Sid": "AllowUserToLoginAsThemselves",
            "Effect": "Allow",
            "Action": [
              "sts:GetFederationToken"
            ],
            "Resource": [
                "arn:aws:sts::${local.core_account_id}:federated-user/$${aws:username}"
            ]
        },
        {
            "Sid": "AllowUserToListTheirOwnMFA",
            "Effect": "Allow",
            "Action": [
                "iam:ListMFADevices"
            ],
            "Resource": [
                "arn:aws:iam::${local.core_account_id}:mfa/*",
                "arn:aws:iam::${local.core_account_id}:user/$${aws:username}"
            ]
        },
        {
            "Sid": "AllowUsersToManageTheirOwnMFA",
            "Effect": "Allow",
            "Action": [
                "iam:CreateVirtualMFADevice",
                "iam:DeleteVirtualMFADevice",
                "iam:EnableMFADevice",
                "iam:ResyncMFADevice"
            ],
            "Resource": [
                "arn:aws:iam::${local.core_account_id}:mfa/$${aws:username}",
                "arn:aws:iam::${local.core_account_id}:user/$${aws:username}"
            ]
        },
        {
            "Sid": "AllowUsersToListAccounts",
            "Effect": "Allow",
            "Action": [
                "iam:GetAccountPasswordPolicy",
                "iam:GetAccountSummary",
                "iam:ListAccountAliases",
                "iam:ListUsers",
                "iam:ListVirtualMFADevices"
            ],
            "Resource": "*"
        },
        {
            "Sid": "AllowUsersToListRoles",
            "Effect": "Allow",
            "Action": [
                "iam:ListRoles"
            ],
            "Resource": [
                "arn:aws:iam::${local.core_account_id}:role/"
            ]
        }
    ]
}
EOF
}

# give admin users access to the state bucket
# (on all accounts)
data aws_iam_policy_document bucket_policy {
  dynamic statement {
    for_each = local.accounts

    content {
      sid       = "put-state-${local.accounts[statement.key].name}"
      resources = ["arn:aws:s3:::${var.state_bucket}/${local.accounts[statement.key].name}.tfstate"]
      actions   = ["s3:PutObject"]

      principals {
        type        = "AWS"
        identifiers = ["arn:aws:iam::${local.accounts[statement.key].id}:role/${var.admin_role}"]
      }
    }
  }

  dynamic statement {
    for_each = local.accounts

    content {
      sid       = "list-bucket-${local.accounts[statement.key].name}"
      resources = ["arn:aws:s3:::${var.state_bucket}"]
      actions   = ["s3:ListBucket"]

      principals {
        type        = "AWS"
        identifiers = ["arn:aws:iam::${local.accounts[statement.key].id}:role/${var.admin_role}"]
      }
    }
  }

  depends_on = [aws_organizations_account.account]
}

resource aws_s3_bucket_policy terraform_state {
  bucket = var.state_bucket
  policy = data.aws_iam_policy_document.bucket_policy.json

  depends_on = [aws_organizations_account.account]
}

# this lets a user assume role into a AdminUser
data aws_iam_policy_document admin_assume_role {
  dynamic statement {
    # loop over each account and create an AdminUser IAM policy document
    for_each = local.accounts

    content {
      actions   = ["sts:assumeRole"]
      resources = ["arn:aws:iam::${local.accounts[statement.key].id}:role/${var.admin_role}"]
    }
  }

  depends_on = [aws_organizations_account.account]
}

# create a new policy with the assume_role permissions
resource aws_iam_policy admin_assume_role {
  name   = "AllowAdminsToAssumeRoleInAccounts"
  policy = data.aws_iam_policy_document.admin_assume_role.json

  depends_on = [aws_organizations_account.account]
}

# this lets a user assume role into a DeveloperUser
data aws_iam_policy_document developer_assume_role {
  dynamic statement {
    for_each = local.accounts

    content {
      actions   = ["sts:assumeRole"]
      resources = ["arn:aws:iam::${local.accounts[statement.key].id}:role/${var.developer_role}"]
    }
  }

  depends_on = [aws_organizations_account.account]
}

# create a new policy with the assume_role permissions
resource aws_iam_policy developer_assume_role {
  name   = "AllowDevelopersToAssumeRoleInAccounts"
  policy = data.aws_iam_policy_document.developer_assume_role.json

  depends_on = [aws_organizations_account.account]
}
