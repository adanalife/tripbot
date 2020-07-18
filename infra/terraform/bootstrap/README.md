## In the beginning...

Before you can start running this infrastructure on a brand new account, you must first prepare the account.
This directory contains the code to bootstrap a new AWS account, including the S3 bucket, a DynamoDB table used as a mutex, and the initial admin accounts.
See `terraform.tfvars` for configuration details.

```bash
# temporarily add the root accounts credentials to aws-vault
# (get them from the IAM page on the AWS console)
aws-vault add adanalife-root
# run the terraform bootstrap code
cd terraform/bootstrap
aws-vault exec adanalife-root -- terraform init
aws-vault exec adanalife-root -- terraform plan -out bootstrap.plan
aws-vault exec adanalife-root -- terraform apply bootstrap.plan
# remove the root account credentials
aws-vault remove adanalife-root
```

### Why is there a tfstate checked in here?

We use S3 to keep track of tfstate for all of our accounts.
In order to do so, we need to create an S3 bucket to store the statefiles.
That's one of the things this bootstrap code is doing... it creates the bucket to store all the state files!

The reason we check in the `.tfstate` file in here here is it's possible we might want to make a change to these resources.
If anything changes in bootstrap, we can re-run terraform using the original state file (and then check in the updated state file... it should be small).


### Learn more

The original design of this repo, including most of this bootstrap code, was taken from [this blog article](https://medium.com/faun/how-i-manage-my-aws-accounts-with-terraform-f52c63dd2aa).
