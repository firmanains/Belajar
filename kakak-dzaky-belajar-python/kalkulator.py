angka1 = int(input("masukkan angka 1 = "))
print(angka1)
angka2 = int(input("masukkan angka 2 = "))
print(angka2)
print("DAFTAR OPERASI")
print("1. Tambah")
print("2. Kurang")
print("3. Bagi")
print("4. Kali")
operasi = input("pilih menu keberapa (1 / 2 / 3 / 4): ")
if operasi == "1": 
    print(angka1+angka2)
elif operasi == "2":
    print(angka1-angka2)
elif operasi == "3":
    print(angka1/angka2)
elif operasi == "4":
    print(angka1*angka2)    
    
