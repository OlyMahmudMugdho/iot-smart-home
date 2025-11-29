#define DECODE_NEC
#include <IRremote.hpp>
#include <DHT.h>
#include <WiFi.h>
#include <PubSubClient.h>

// --- Pins (adjust if needed) ---
constexpr uint8_t IR_RECV_PIN {3};
constexpr uint8_t RELAY_PIN {2};      // IR controlled LED via relay
constexpr uint8_t LED2_PIN {12};      // Room light controlled by sensors
constexpr uint8_t LDR_PIN {13};       // Analog pin (LDR)
constexpr uint8_t DHT_PIN {37};       // DHT22
constexpr uint8_t MQ2_PIN {10};       // MQ-2 gas sensor
constexpr uint8_t TRIG1_PIN {8};
constexpr uint8_t ECHO1_PIN {9};
constexpr uint8_t TRIG2_PIN {6};
constexpr uint8_t ECHO2_PIN {7};
constexpr uint8_t BUZZER_PIN {5};

constexpr uint16_t TOGGLE_BTN = 0x68;
const int LDR_THRESHOLD = 2000;
const int MQ2_THRESHOLD = 3000;
const int BUZZER_FREQ = 2000;
#define DHTTYPE DHT22
DHT dht(DHT_PIN, DHTTYPE);

// Person counting
volatile int personCount = 0;
unsigned long lastActivate1 = 0;
unsigned long lastActivate2 = 0;
bool lastState1 = false;
bool lastState2 = false;
const long DISTANCE_TRIGGER_CM = 200;
const unsigned long PAIR_WINDOW_MS = 70000;
const unsigned long EDGE_DEBOUNCE_MS = 200;

// WiFi & MQTT
const char* ssid = "Wokwi-GUEST";
const char* password = "";
const char* mqttServer = "broker.hivemq.com";
const uint16_t mqttPort = 1883;
const char* mqttTopicRelay = "myhome/room/relay/set";
const char* mqttTopicMetrics = "myhome/room/metrics";

WiFiClient espClient;
PubSubClient client(espClient);

// --- Helper functions ---
long getDistancePins(uint8_t trigPin, uint8_t echoPin) {
  digitalWrite(trigPin, LOW);
  delayMicroseconds(2);
  digitalWrite(trigPin, HIGH);
  delayMicroseconds(10);
  digitalWrite(trigPin, LOW);
  long duration = pulseIn(echoPin, HIGH, 30000);
  if (duration == 0) return -1;
  return duration * 0.034 / 2;
}

void mqttCallback(char* topic, byte* payload, unsigned int length) {
  String msg;
  for (unsigned int i = 0; i < length; i++) msg += (char)payload[i];
  msg.trim();

  if (msg == "ON") digitalWrite(RELAY_PIN, HIGH);
  else if (msg == "OFF") digitalWrite(RELAY_PIN, LOW);
  else if (msg == "TOGGLE") digitalWrite(RELAY_PIN, !digitalRead(RELAY_PIN));
}

void reconnectMQTT() {
  while (!client.connected()) {
    if (client.connect("ESP32_Relay")) client.subscribe(mqttTopicRelay);
    else delay(2000);
  }
}

void handleUltrasonicPairing(bool rising1, bool rising2) {
  unsigned long now = millis();
  if (rising1) {
    lastActivate1 = now;
    if (lastActivate2 > 0 && (now - lastActivate2) <= PAIR_WINDOW_MS && lastActivate2 + EDGE_DEBOUNCE_MS < now) {
      if (lastActivate2 < lastActivate1) { if (personCount > 0) personCount--; lastActivate1 = lastActivate2 = 0; }
    }
  }
  if (rising2) {
    lastActivate2 = now;
    if (lastActivate1 > 0 && (now - lastActivate1) <= PAIR_WINDOW_MS && lastActivate1 + EDGE_DEBOUNCE_MS < now) {
      if (lastActivate1 < lastActivate2) { personCount++; lastActivate1 = lastActivate2 = 0; }
    }
  }
}

// Publish metrics
void publishMetrics(int ldr, long d1, long d2, int mq2Value, float temp, float hum, int count) {
  String payload = "{";
  payload += "\"LDR\":" + String(ldr);
  payload += ",\"Dist1\":" + String(d1);
  payload += ",\"Dist2\":" + String(d2);
  payload += ",\"MQ2\":" + String(mq2Value);
  payload += ",\"Temp\":" + String(temp);
  payload += ",\"Hum\":" + String(hum);
  payload += ",\"Count\":" + String(personCount);
  payload += "}";
  client.publish(mqttTopicMetrics, payload.c_str());
}

// --- Setup ---
void setup() {
  Serial.begin(115200);
  pinMode(RELAY_PIN, OUTPUT); digitalWrite(RELAY_PIN, LOW);
  pinMode(LED2_PIN, OUTPUT); digitalWrite(LED2_PIN, LOW);
  pinMode(MQ2_PIN, INPUT);
  pinMode(TRIG1_PIN, OUTPUT); pinMode(ECHO1_PIN, INPUT);
  pinMode(TRIG2_PIN, OUTPUT); pinMode(ECHO2_PIN, INPUT);
  pinMode(BUZZER_PIN, OUTPUT); digitalWrite(BUZZER_PIN, LOW);

  IrReceiver.begin(IR_RECV_PIN);
  dht.begin();

  WiFi.begin(ssid, password);
  while (WiFi.status() != WL_CONNECTED) delay(500);

  client.setServer(mqttServer, mqttPort);
  client.setCallback(mqttCallback);
  reconnectMQTT();
}

// --- Loop ---
void loop() {
  if (!client.connected()) reconnectMQTT();
  client.loop();

  // IR control
  if (IrReceiver.decode()) {
    if (IrReceiver.decodedIRData.command == TOGGLE_BTN)
      digitalWrite(RELAY_PIN, !digitalRead(RELAY_PIN));
    IrReceiver.resume();
  }

  // Sensors
  int ldrValue = analogRead(LDR_PIN);
  long dist1 = getDistancePins(TRIG1_PIN, ECHO1_PIN);
  long dist2 = getDistancePins(TRIG2_PIN, ECHO2_PIN);
  int mq2Value = analogRead(MQ2_PIN);
  float t = dht.readTemperature();
  float h = dht.readHumidity();

  bool objectClose1 = (dist1 > 0 && dist1 < DISTANCE_TRIGGER_CM);
  bool objectClose2 = (dist2 > 0 && dist2 < DISTANCE_TRIGGER_CM);
  unsigned long now = millis();
  bool rising1 = objectClose1 && !lastState1 && (now - lastActivate1 > EDGE_DEBOUNCE_MS);
  bool rising2 = objectClose2 && !lastState2 && (now - lastActivate2 > EDGE_DEBOUNCE_MS);
  lastState1 = objectClose1; lastState2 = objectClose2;
  if (rising1 || rising2) handleUltrasonicPairing(rising1, rising2);
  if (personCount < 0) personCount = 0;

  if (personCount > 0 && ldrValue > LDR_THRESHOLD) digitalWrite(LED2_PIN, HIGH);
  else digitalWrite(LED2_PIN, LOW);

  if (mq2Value > MQ2_THRESHOLD) tone(BUZZER_PIN, BUZZER_FREQ);
  else noTone(BUZZER_PIN);

  // Publish metrics over MQTT
  publishMetrics(ldrValue, dist1, dist2, mq2Value, t, h, personCount);

  delay(500); // slower update
}
