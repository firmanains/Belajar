package lab12

// File ini sudah jadi. Jangan diubah; baca saja.

// Schema berisi dua read model dan tabel pendukungnya. Tidak ada tabel write
// model di lab ini: read model dibangun hanya dari event yang datang.
const Schema = `
-- Data referensi: jumlah kamar per room type.
CREATE TABLE room_types (
	room_type text PRIMARY KEY,
	capacity  int  NOT NULL
);

-- Read model 1: daftar booking untuk layar resepsionis. Satu baris per booking.
CREATE TABLE booking_list_view (
	booking_id text PRIMARY KEY,
	guest_name text NOT NULL,
	room_type  text NOT NULL,
	check_in   date NOT NULL,
	check_out  date NOT NULL,
	rooms      int  NOT NULL,
	status     text NOT NULL
);

-- Read model 2: kamar terjual per room type per malam, untuk tape chart.
-- Satu baris per (room_type, day). Jawaban sudah dihitung saat event datang.
CREATE TABLE availability_view (
	room_type  text NOT NULL,
	day        date NOT NULL,
	rooms_sold int  NOT NULL,
	PRIMARY KEY (room_type, day)
);

-- Event yang sudah diproyeksikan. Diisi dalam transaction yang sama dengan
-- perubahan read model.
CREATE TABLE processed_events (
	event_id     text PRIMARY KEY,
	processed_at timestamptz NOT NULL DEFAULT now()
);
`
