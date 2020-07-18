# these are used when creating new accounts
# i.e. danadotlol+account_name@gmail.com
email_prefix = "danadotlol+"
email_domain = "gmail.com"

# this is where the core accounts Terraform state will live
state_bucket = "adanalife-core-tf-state"

# note that the order of these matters, and if you remove them
# you will have to manually mess with the terraform state
account_names = [
  "stage-1"
]
