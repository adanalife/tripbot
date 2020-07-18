```bash
cat ~/.aws/config

[profile adanalife-core]
region=us-east-1
source_profile=adanalife

[profile adanalife-stage]
region=us-east-1
source_profile=adanalife
role_arn=arn:aws:iam::413585268653:role/AdminUser

[profile adanalife-stage-developer]
region=us-east-1
source_profile=adanalife
role_arn=arn:aws:iam::413585268653:role/DeveloperUser
```


```bash
cat ~/.bash_profile.local

alias aws-dana-core="aws-vault exec adanalife-core --no-session"
alias aws-dana-stage="aws-vault exec adanalife-stage --no-session"
alias aws-dana-stage-developer="aws-vault exec adanalife-stage-developer --no-session"
# alias aws-dana-core-root="aws-vault exec adanalife-root --no-session"

alias login-dana-core="aws-vault login adanalife-core"
alias login-dana-stage="aws-vault login adanalife-stage"
alias login-dana-stage-developer="aws-vault login adanalife-stage-developer --duration=12h"
# alias login-dana-root="aws-vault login adanalife-root"

alias tf-dana-core="cd ~/danalol/tripbot/infra/terraform/core && aws-dana-core"
alias tf-dana-stage="cd ~/danalol/tripbot/infra/terraform/stage-1 && aws-dana-stage"
```
