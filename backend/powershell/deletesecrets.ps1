# List all secrets and delete them
function Remove-AllGitHubSecrets {
    Write-Host "Fetching all secrets..." -ForegroundColor Yellow
    
    # Get all secrets
    $secrets = gh secret list 2>&1
    
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Error fetching secrets: $secrets" -ForegroundColor Red
        exit 1
    }

    # Counter for tracking progress
    $total = ($secrets | Measure-Object -Line).Lines
    $current = 0

    # Process each secret
    foreach ($secret in $secrets) {
        # Skip empty lines
        if ([string]::IsNullOrWhiteSpace($secret)) {
            continue
        }

        # Extract secret name (first column from gh secret list output)
        $secretName = ($secret -split '\s+')[0]
        $current++

        Write-Host "($current/$total) Deleting secret: $secretName" -ForegroundColor Yellow
        
        try {
            $result = gh secret delete $secretName 2>&1
            if ($LASTEXITCODE -eq 0) {
                Write-Host "Successfully deleted secret: $secretName" -ForegroundColor Green
            } else {
                Write-Host "Failed to delete secret: $secretName - $result" -ForegroundColor Red
            }
        }
        catch {
            Write-Host "Error deleting secret: $secretName - $_" -ForegroundColor Red
        }
    }

    Write-Host "Completed deleting secrets" -ForegroundColor Green
}

# Run the function
Remove-AllGitHubSecrets