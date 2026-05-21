import os

files = [
    "README.md",
    "architecture.md",
    "security.md",
    "ftp-val.md",
    "api.md",
    "database.md",
    "products.md",
    "logic.md"
]

with open("Busk_Seguros_Manual_Tecnico.md", "w", encoding="utf-8") as out:
    for filename in files:
        if os.path.exists(filename):
            with open(filename, "r", encoding="utf-8") as f:
                content = f.read()
                out.write(content + "\n\n")

print("Docs concatenados sin hr extra en Busk_Seguros_Manual_Tecnico.md")
