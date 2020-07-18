# load in data from json files
locals {
  user_data  = yamldecode(file("user_data.yml"))
  group_data = yamldecode(file("group_data.yml"))
}

# loop over user_data and create the users
resource aws_iam_user user {
  count = length(local.user_data)
  name  = local.user_data[count.index]["user"]
  tags = {
    Name = local.user_data[count.index]["user"]
  }
  force_destroy = false
}

# loop over group_data and put developers in Developer group
resource aws_iam_group_membership developers {
  name  = "developers-group-membership"
  users = local.group_data[aws_iam_group.developer.name]
  group = aws_iam_group.developer.name
}
