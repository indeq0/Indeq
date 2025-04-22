# Load the YAML file and extract the data section
$data = yq eval '.' .\backend-config.yaml

# Check if any secrets were found
if ($data.Count -eq 0) {
    Write-Host "No secrets found in backend-config.yaml."
    exit
}

# Loop through each secret
foreach ($secret in $data) {
    # Extract the raw secret string
    $rawSecret = $secret

    # Trim the string and extract key and value
    $trimmedSecret = $rawSecret.Trim()
    $keyValue = $trimmedSecret -split ': '

    # Check if the keyValue array has the expected number of elements
    if ($keyValue.Count -lt 2) {
        Write-Host "Warning: The entry '$rawSecret' does not contain a valid key-value pair. Skipping..."
        continue
    }

    # Extract the key and value
    $name = $keyValue[0].Trim()  # This will be the key
    $value = $keyValue[1].Trim()  # This will be the value if it exists

    # For EC2_KEY_PAIR, ensure proper line endings
    if ($name -eq 'EC2_KEY_PAIR') {
        # Read the key file
        $keyContent = $value
        
        # Split the key into parts
        $beginMarker = "-----BEGIN RSA PRIVATE KEY-----"
        $endMarker = "-----END RSA PRIVATE KEY-----"
        $keyBody = $keyContent -replace $beginMarker, "" -replace $endMarker, "" -replace "\s+", ""
    
        
        # Reconstruct the key with proper format
        $value = $keyBody
    }
    # Debug output to check the extracted values
    $value = $value -replace '^"|"$', ''  # Remove extra outer double quotes from the value
    Write-Host "Extracted secret: Name='$name', Value='$value'"
    # Check if the value is empty or commented out
    if (-not $value -or $value -match '^\s*#') {
        Write-Host "Warning: The value for secret '$name' is empty or commented out. Skipping..."
        continue
    }


    # Set the secret in GitHub
    gh secret set $name --body $value

}