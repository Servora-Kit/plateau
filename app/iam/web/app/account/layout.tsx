import { AccountLayoutFrame } from "@/components/account-shell"
import { AccountProfileProvider } from "@/hooks/use-profile"

export default function AccountLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <AccountProfileProvider>
      <AccountLayoutFrame>{children}</AccountLayoutFrame>
    </AccountProfileProvider>
  )
}
