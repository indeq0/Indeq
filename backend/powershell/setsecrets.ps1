# Path to .env file
$envPath = "../common/config/.env"

# Function to set GitHub secret
function Set-GitHubSecret($name, $value) {
    Write-Host "Setting secret: $name"
    try {
        $result = gh secret set $name --body $value 2>&1
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Success: $name"
        } else {
            Write-Host "Failed: $name"
        }
    } catch {
        Write-Host "Error: $name"
    }
}

# Main script
if (Test-Path $envPath) {
    Write-Host "Reading .env file..."
    try {
        # Read the entire .env file content
        $envContent = Get-Content -Path $envPath -Raw
        
        # Set the entire content as one secret
        Set-GitHubSecret "ENV_FILE" $envContent
        
    } catch {
        Write-Host "Error: Failed to read .env file"
        Write-Host $_
    }
} else {
    Write-Host "Error: .env file not found at $envPath"
}