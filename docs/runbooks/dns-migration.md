# Cloudflare DNS migration runbook

Nameserver migration is a controlled change. Incomplete DNS records can interrupt the landing page, applicant portal, API, Clerk, or email.

## Inventory and parity

1. Export every GoDaddy record and independently query the current authoritative nameservers.
2. Include apex, `www`, `apply`, `api`, MX, SPF, DKIM, DMARC, Clerk, Vercel, DigitalOcean, and all domain-verification records in `global/dns_records`.
3. Compare record name, type, content, priority, and TTL. Preserve unknown records until their owner confirms removal.
4. Apply the Cloudflare zone with DNSSEC disabled. Keep Vercel verification records DNS-only. Proxy only API records that have been tested through Cloudflare.
5. From two public resolvers, verify the new Cloudflare zone answers match GoDaddy.

## Cutover

1. Lower relevant TTLs at least one old-TTL interval before the change.
2. Disable and remove any legacy DS record at the registrar; verify the chain is unsigned.
3. Change registrar nameservers to the exact Cloudflare nameservers from Terraform output.
4. Poll NS, A/AAAA/CNAME, MX, TXT, and CAA responses until public resolvers converge.
5. Test `hackatlantic.ca`, `www`, `apply`, `api/readyz`, Clerk sign-in/callback, inbound mail, outbound SPF/DKIM, and DMARC alignment.
6. Enable Cloudflare DNSSEC, publish its DS record at the registrar, and verify the signed chain.
7. Restore normal TTLs and attach redacted parity evidence to the change record.

If the landing site, API, Clerk, or mail fails, restore the prior nameservers and legacy DS state as appropriate. Never improvise DNSSEC changes during an outage.
