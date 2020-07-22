# manage the dana.lol domain
resource aws_route53_zone primary {
  name = var.domain
}

#TODO: figure out how to do aliases
# www.dana.lol.  A ALIAS d6kb0mm00m70t.cloudfront.net.
# resource aws_route53_record www {
#   zone_id = aws_route53_zone.primary.zone_id
#   name    = "www.${var.domain}"
#   type    = "A"
#   ttl     = "300"

#   alias {
#     name                   = "${aws_elb.main.dns_name}"
#     zone_id                = "${aws_elb.main.zone_id}"
#     evaluate_target_health = true
#   }
# }


#TODO: is this an A alias?
# dana.lol ALIAS www.dana.lol.
# resource aws_route53_record naked {
#   zone_id = aws_route53_zone.primary.zone_id
#   name    = "www.${var.domain}"
#   type    = "A"
#   ttl     = "300"
#   records = ["${aws_eip.lb.public_ip}"]
# }

#TODO: create this as an alias
# staging.dana.lol.  ALIAS A dykrdvs8xqodx.cloudfront.net. 

resource aws_route53_record status {
  zone_id = aws_route53_zone.primary.zone_id
  name    = "status.${var.domain}"
  type    = "CNAME"
  ttl     = "300"
  records = ["stats.uptimerobot.com"]
}

resource aws_route53_record keybase {
  zone_id = aws_route53_zone.primary.zone_id
  name    = "_keybase.${var.domain}"
  type    = "TXT"
  ttl     = "300"
  records = ["keybase-site-verification=4c5lF70z6Zp4jBKt7lDhS9PT-fJ5xFTip_2H_qBkZ1c"]
}

resource aws_route53_record develop {
  zone_id = aws_route53_zone.primary.zone_id
  name    = "develop.${var.domain}"
  type    = "CNAME"
  ttl     = "300"
  records = ["localhost"]
}


resource aws_route53_record tripbot {
  zone_id = aws_route53_zone.primary.zone_id
  name    = "tripbot.${var.domain}"
  type    = "A"
  ttl     = "300"
  records = ["172.3.109.123"]
}

resource aws_route53_record certbot {
  zone_id = aws_route53_zone.primary.zone_id
  name    = "_acme-challenge.${var.domain}"
  type    = "TXT"
  ttl     = "300"
  records = ["3DnnRt02WD645OYeOEAuR2cw7--WiWT3YSP_RMlaNu0"]
}

#TODO: is this being used anywhere?
# resource aws_route53_record twitch_scripts {
#   zone_id = aws_route53_zone.primary.zone_id
#   name    = "twitch-scripts.${var.domain}"
#   type    = "A"
#   ttl     = "300"
#   records = ["172.3.109.123"]
# }
