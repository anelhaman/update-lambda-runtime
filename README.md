# update-lambda-runtime (Go + Cobra)

Manage and upgrade AWS Lambda runtimes across **accounts** (via profiles) and **regions**.
Designed to help you list current runtimes and bump Python Lambdas from **python3.9 → python3.12** safely.

## ✨ Features

- **List** functions with current runtime across one or more regions
- **Bump** runtime from a source to a target (defaults: `python3.9` → `python3.12`)
- Works cross-account via `~/.aws/config` **profiles**
- Built with **Cobra** (nice help/UX) and **AWS SDK for Go v2**
- Waits for update completion and reports success/failure

> Tip: Use `list` first (safe/dry). Then run `bump` once you’re happy.

---

## 📦 Requirements

- **Go** 1.20+ (recommend 1.21+)
- AWS credentials/profile configured in `~/.aws/config` and `~/.aws/credentials`
- IAM permissions:
  - `lambda:ListFunctions`
  - `lambda:GetFunctionConfiguration`
  - `lambda:UpdateFunctionConfiguration`

Example minimal policy (attach to the role used by your profile):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "LambdaReadWriteRuntimes",
      "Effect": "Allow",
      "Action": [
        "lambda:ListFunctions",
        "lambda:GetFunctionConfiguration",
        "lambda:UpdateFunctionConfiguration"
      ],
      "Resource": "*"
    }
  ]
}
```

---

## 🚀 Install

```bash
mkdir update-lambda-runtime && cd update-lambda-runtime
go mod init example.com/update-lambda-runtime

go get github.com/aws/aws-sdk-go-v2@v1
go get github.com/aws/aws-sdk-go-v2/config@v1
go get github.com/aws/aws-sdk-go-v2/service/lambda@v1
go get github.com/spf13/cobra@v1

go build -o update-lambda-runtime
```

---

## 🧭 Commands

### list (safe)
```bash
./update-lambda-runtime list --profile otheracct --regions ap-southeast-1,us-east-1 --all
```

### bump
```bash
./update-lambda-runtime bump --profile otheracct --regions ap-southeast-1 --function my-func
```
Or bump all:
```bash
./update-lambda-runtime bump --profile otheracct --regions us-east-1 --all
```

---

## 🔧 Global Flags

| Flag | Type | Default | Description |
|---|---|---:|---|
| `--profile` | string | (required) | AWS profile from `~/.aws/config` |
| `--regions` | string slice | (required) | Comma-separated or repeat flag |
| `--function` | string |  | Single Lambda name (use instead of `--all`) |
| `--all` | bool | `false` | Process all functions in region(s) |
| `--source-runtime` | string | `python3.9` | Source runtime |
| `--target-runtime` | string | `python3.12` | Target runtime |
| `--wait-timeout` | duration | `5m` | Max wait per update |
| `--wait-interval` | duration | `5s` | Polling interval |

---

## 🧪 Examples

List all functions:
```bash
./update-lambda-runtime list --profile otheracct --regions ap-southeast-1,us-east-1 --all
```

Bump a single function:
```bash
./update-lambda-runtime bump --profile otheracct --regions ap-southeast-1 --function my-func
```

Bump all 3.9 → 3.12:
```bash
./update-lambda-runtime bump --profile otheracct --regions us-east-1 --all
```

---

## 🖨 Output

```
Profile              Region            FunctionName                                                     CurrentRuntime
-------              ------            ------------                                                     --------------
otheracct            ap-southeast-1    my-func                                                          python3.9
otheracct            us-east-1         another-func                                                     python3.12
```

---

## ⚠️ Notes

- **Layers**: May need 3.12 versions.
- **Code/deps**: Rebuild for 3.12 if needed.
- **Aliases**: Only updates unpublished config.
- **Permissions**: Ensure correct IAM policy.
- **Regions**: Multiple allowed.

---

## 🔍 Troubleshooting

- AccessDeniedException → Check IAM policy/profile
- ResourceNotFoundException → Wrong name/region/profile
- ThrottlingException → Large fleets, rerun with delays
- Update failure → Check LastUpdateStatusReason

---

## 🛠 Extending

- Concurrency with goroutines
- Multi-profile loop
- Dry-run mode for bump

---

## 🧾 License

MIT
