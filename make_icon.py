from PIL import Image
import sys

src = r"C:\Users\ASUS\.gemini\antigravity\brain\202cb724-2fc2-47b6-9ffe-af4f20a1b3ff\scheduler_icon_1784664430416.png"
dst = r"e:\exe\scheduler.ico"

img = Image.open(src).convert("RGBA")
sizes = [(16,16),(32,32),(48,48),(64,64),(128,128),(256,256)]
imgs  = [img.resize(s, Image.LANCZOS) for s in sizes]
imgs[0].save(dst, format="ICO", sizes=sizes, append_images=imgs[1:])
print("Icon saved:", dst)
