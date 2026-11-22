nilai = [80, 60, 75, 90, 55, 78]
total = 0

for i in range(len(nilai)):
    if nilai[i] < 75:
        total = total + 1

print(total)