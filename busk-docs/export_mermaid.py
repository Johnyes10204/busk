import re
import subprocess
import os

with open('/Users/john/Desktop/buskseguros-design/busk-docs/architecture.md', 'r') as f:
    content = f.read()

pattern = r'```mermaid\n(.*?)```'
matches = re.findall(pattern, content, re.DOTALL)

for i, match in enumerate(matches):
    mmd_filename = f'diagram_{i}.mmd'
    png_filename = f'diagram_{i}.png'
    with open(mmd_filename, 'w') as f:
        f.write(match.strip())
    
    print(f"Generating {png_filename}...")
    subprocess.run(["npx", "-y", "@mermaid-js/mermaid-cli", "-i", mmd_filename, "-o", f"/Users/john/Desktop/buskseguros-design/busk-docs/assets/{png_filename}"])

print("Done")
