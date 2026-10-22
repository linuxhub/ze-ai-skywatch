package appidentity

import (
	"context"
	"encoding/json"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func inspectPlatform(ctx context.Context, info *Info) {
	script := `[Console]::InputEncoding=[Text.Encoding]::UTF8; [Console]::OutputEncoding=[Text.Encoding]::UTF8; $p=[Console]::In.ReadToEnd(); $v=[Diagnostics.FileVersionInfo]::GetVersionInfo($p); $s=''; try { $c=New-Object Security.Cryptography.X509Certificates.X509Certificate2 ([Security.Cryptography.X509Certificates.X509Certificate]::CreateFromSignedFile($p)); $s=$c.GetNameInfo([Security.Cryptography.X509Certificates.X509NameType]::SimpleName,$false) } catch {}; @{app_name_declared=$v.ProductName;version_declared=$v.ProductVersion;company_declared=$v.CompanyName;signer_declared=$s} | ConvertTo-Json -Compress`
	data, err := command(ctx, strings.NewReader(info.Executable), "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err == nil {
		var metadata Info
		if json.Unmarshal(data, &metadata) == nil {
			info.AppName, info.Version, info.DeclaredCompany, info.Signer = metadata.AppName, metadata.Version, metadata.DeclaredCompany, metadata.Signer
		}
	}
	filename, err := windows.UTF16PtrFromString(info.Executable)
	if err != nil || ctx.Err() != nil {
		return
	}
	file := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: filename}
	trust := windows.WinTrustData{
		Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE,
		RevocationChecks: windows.WTD_REVOKE_NONE, UnionChoice: windows.WTD_CHOICE_FILE,
		StateAction: windows.WTD_STATEACTION_VERIFY, ProvFlags: windows.WTD_CACHE_ONLY_URL_RETRIEVAL | windows.WTD_REVOCATION_CHECK_NONE,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&file),
	}
	err = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &trust)
	trust.StateAction = windows.WTD_STATEACTION_CLOSE
	windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &trust)
	if err == nil {
		info.Signature = "磁盘文件签名校验通过（离线；未检查吊销）"
	} else {
		info.Signature = "无签名、离线信任不足或校验未通过"
	}
}
