#define DECODE_NEC
#include <IRremote.hpp>
#include <DHT.h>

// --- Pins (adjust if needed) ---
constexpr uint8_t IR_RECV_PIN {3};
constexpr uint8_t RELAY_PIN {2};      // IR controlled LED via relay
constexpr uint8_t LED2_PIN {12};      // Room light controlled by sensors

constexpr uint8_t LDR_PIN {13};       // Analog pin (LDR)
constexpr uint8_t DHT_PIN {37};       // DHT22
constexpr uint8_t MQ2_PIN {10};       // MQ-2 gas sensor

// --- Sensors ---
// constexpr uint8_t PIR_PIN {14};       // PIR OUT pin

// Ultrasonic sensor 1 (door-side sensor 1)
constexpr uint8_t TRIG1_PIN {8};
constexpr uint8_t ECHO1_PIN {9};

// Ultrasonic sensor 2 (door-side sensor 2)
constexpr uint8_t TRIG2_PIN {6};
constexpr uint8_t ECHO2_PIN {7};

constexpr uint16_t TOGGLE_BTN = 0x68;
const int LDR_THRESHOLD = 2000; // calibrate to your LDR and ADC range


// buzzer config
constexpr uint8_t BUZZER_PIN {5};  // connect buzzer to digital pin 5
const int MQ2_THRESHOLD = 3000;     // adjust threshold for gas detection
const int BUZZER_FREQ = 2000;      // buzzer frequency in Hz


#define DHTTYPE DHT22
DHT dht(DHT_PIN, DHTTYPE);

// Person counting logic
volatile int personCount = 0;

// timing / debounce
unsigned long lastActivate1 = 0;
unsigned long lastActivate2 = 0;
bool lastState1 = false;
bool lastState2 = false;

// ADJUSTED for slider testing:
const long DISTANCE_TRIGGER_CM = 200;   // use 200 cm for Wokwi slider-friendly testing
const unsigned long PAIR_WINDOW_MS = 70000; // 7 seconds pairing window (slower testing)
const unsigned long EDGE_DEBOUNCE_MS = 200; // ignore bounce within 200ms

// helper to measure one ultrasonic sensor (non-blocking wrapper)
long getDistancePins(uint8_t trigPin, uint8_t echoPin) {
  digitalWrite(trigPin, LOW);
  delayMicroseconds(2);
  digitalWrite(trigPin, HIGH);
  delayMicroseconds(10);
  digitalWrite(trigPin, LOW);

  // pulseIn with timeout to avoid lockups (30 ms)
  long duration = pulseIn(echoPin, HIGH, 30000);
  if (duration == 0) return -1; // timeout / no echo
  long distance = duration * 0.034 / 2; // cm
  return distance;
}

void setup() {
  Serial.begin(115200);

  // --- Relay LED ---
  pinMode(RELAY_PIN, OUTPUT);
  digitalWrite(RELAY_PIN, LOW);

  // --- LED2 ---
  pinMode(LED2_PIN, OUTPUT);
  digitalWrite(LED2_PIN, LOW);

  // --- IR ---
  IrReceiver.begin(IR_RECV_PIN);

  // --- DHT ---
  dht.begin();

  // --- MQ-2 ---
  pinMode(MQ2_PIN, INPUT);

  // --- PIR + Ultrasonic setup ---
  // pinMode(PIR_PIN, INPUT);

  pinMode(TRIG1_PIN, OUTPUT);
  pinMode(ECHO1_PIN, INPUT);

  pinMode(TRIG2_PIN, OUTPUT);
  pinMode(ECHO2_PIN, INPUT);


  // buzzer setup
  pinMode(BUZZER_PIN, OUTPUT);
digitalWrite(BUZZER_PIN, LOW);


  Serial.println("System ready");
}

void handleUltrasonicPairing(bool rising1, bool rising2) {
  unsigned long now = millis();

  if (rising1) {
    lastActivate1 = now;
    // If sensor2 fired recently and before sensor1 => exit
    if (lastActivate2 > 0 && (now - lastActivate2) <= PAIR_WINDOW_MS && lastActivate2 + EDGE_DEBOUNCE_MS < now) {
      if (lastActivate2 < lastActivate1) {
        if (personCount > 0) personCount--;
        Serial.print("Exit detected. Count: ");
        Serial.println(personCount);
        lastActivate1 = lastActivate2 = 0;
      }
    }
  }

  if (rising2) {
    lastActivate2 = now;
    // If sensor1 fired recently and before sensor2 => entry
    if (lastActivate1 > 0 && (now - lastActivate1) <= PAIR_WINDOW_MS && lastActivate1 + EDGE_DEBOUNCE_MS < now) {
      if (lastActivate1 < lastActivate2) {
        personCount++;
        Serial.print("Entry detected. Count: ");
        Serial.println(personCount);
        lastActivate1 = lastActivate2 = 0;
      }
    }
  }
}

void loop() {

  // --- IR control ---
  if (IrReceiver.decode()) {
    if (IrReceiver.decodedIRData.command == TOGGLE_BTN) {
      digitalWrite(RELAY_PIN, !digitalRead(RELAY_PIN));
      Serial.println("LED1 toggled");
    }
    IrReceiver.resume();
  }

  // --- LDR ---
  int ldrValue = analogRead(LDR_PIN);
  bool isDark = ldrValue > LDR_THRESHOLD;

  // --- PIR ---
  // int pirState = digitalRead(PIR_PIN);
  // bool motionDetected = pirState == HIGH;

  // --- Ultrasonic sensors ---
  long distance1 = getDistancePins(TRIG1_PIN, ECHO1_PIN);
  long distance2 = getDistancePins(TRIG2_PIN, ECHO2_PIN);

  bool objectClose1 = (distance1 > 0 && distance1 < DISTANCE_TRIGGER_CM);
  bool objectClose2 = (distance2 > 0 && distance2 < DISTANCE_TRIGGER_CM);

  // Rising-edge detection with debounce
  unsigned long now = millis();
  bool rising1 = false;
  bool rising2 = false;

  if (objectClose1 && !lastState1) {
    if (now - lastActivate1 > EDGE_DEBOUNCE_MS) {
      rising1 = true;
    }
  }
  if (objectClose2 && !lastState2) {
    if (now - lastActivate2 > EDGE_DEBOUNCE_MS) {
      rising2 = true;
    }
  }

  // update last states
  lastState1 = objectClose1;
  lastState2 = objectClose2;

  // handle pairing and counting only on detected rising edges
  if (rising1 || rising2) {
    handleUltrasonicPairing(rising1, rising2);
  }

  // Safety: don't allow negative counts
  if (personCount < 0) personCount = 0;

  // --- LED2 logic (room light) ---
  if (personCount > 0 && isDark) {
    digitalWrite(LED2_PIN, HIGH);
  } else {
    digitalWrite(LED2_PIN, LOW);
  }

  // --- Serial status for debugging ---
  Serial.print("LDR: "); Serial.print(ldrValue);
  // Serial.print("  PIR: "); Serial.print(pirState);
  Serial.print("  Dist1: "); Serial.print(distance1);
  Serial.print(" cm  Dist2: "); Serial.print(distance2);
  Serial.print(" cm  Count: "); Serial.println(personCount);

  // --- DHT22 ---
  float t = dht.readTemperature();
  float h = dht.readHumidity();
  if (!isnan(t) && !isnan(h)) {
    Serial.print("Temp: "); Serial.print(t);
    Serial.print("C  Hum: "); Serial.println(h);
  }

  // --- MQ-2 ---
  int mq2 = analogRead(MQ2_PIN);
  Serial.print("MQ2: "); Serial.println(mq2);
  // --- Buzzer logic ---
  if (mq2 > MQ2_THRESHOLD) {
      // Beep buzzer
      tone(BUZZER_PIN, BUZZER_FREQ); // start buzzer at given frequency
  } else {
      noTone(BUZZER_PIN); // stop buzzer
  }

  delay(120); // small delay
}
