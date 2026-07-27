package valobj

import "testing"

func TestNewNaming(t *testing.T) {
	tests := map[string]Naming{
		"Stripe": {
			Pascal: "Stripe",
			Camel:  "stripe",
			Snake:  "stripe",
			Kebab:  "stripe",
			Lower:  "stripe",
			Upper:  "STRIPE",
			Words:  "stripe",
			Plural: "Stripes",
		},
		"payment gateway": {
			Pascal: "PaymentGateway",
			Camel:  "paymentGateway",
			Snake:  "payment_gateway",
			Kebab:  "payment-gateway",
			Lower:  "paymentgateway",
			Upper:  "PAYMENT_GATEWAY",
			Words:  "payment gateway",
			Plural: "PaymentGateways",
		},
		"HTTPCache": {
			Pascal: "HTTPCache",
			Camel:  "httpCache",
			Snake:  "http_cache",
			Kebab:  "http-cache",
			Lower:  "httpcache",
			Upper:  "HTTP_CACHE",
			Words:  "http cache",
			Plural: "HTTPCaches",
		},
		"user_id": {
			Pascal: "UserID",
			Camel:  "userID",
			Snake:  "user_id",
			Kebab:  "user-id",
			Lower:  "userid",
			Upper:  "USER_ID",
			Words:  "user id",
			Plural: "UserIDs",
		},
		"category": {
			Pascal: "Category",
			Camel:  "category",
			Snake:  "category",
			Kebab:  "category",
			Lower:  "category",
			Upper:  "CATEGORY",
			Words:  "category",
			Plural: "Categories",
		},
		"box": {
			Pascal: "Box",
			Camel:  "box",
			Snake:  "box",
			Kebab:  "box",
			Lower:  "box",
			Upper:  "BOX",
			Words:  "box",
			Plural: "Boxes",
		},
		"gateway": {
			Pascal: "Gateway",
			Camel:  "gateway",
			Snake:  "gateway",
			Kebab:  "gateway",
			Lower:  "gateway",
			Upper:  "GATEWAY",
			Words:  "gateway",
			Plural: "Gateways",
		},
		"orderItem-v2": {
			Pascal: "OrderItemV2",
			Camel:  "orderItemV2",
			Snake:  "order_item_v2",
			Kebab:  "order-item-v2",
			Lower:  "orderitemv2",
			Upper:  "ORDER_ITEM_V2",
			Words:  "order item v2",
			Plural: "OrderItemV2s",
		},
		"": {},
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			naming := NewNaming(input)

			if naming != expected {
				t.Errorf("NewNaming(%q)\n got: %+v\nwant: %+v", input, naming, expected)
			}
		})
	}
}

func TestParseField(t *testing.T) {
	tests := []struct {
		input    string
		expected Field
		wantErr  bool
	}{
		{
			input:    "Name:string",
			expected: Field{Name: NewNaming("Name"), Type: "string"},
		},
		{
			input:    "user_id:int",
			expected: Field{Name: NewNaming("user_id"), Type: "int"},
		},
		{
			input:    `Total:float64:json:"total"`,
			expected: Field{Name: NewNaming("Total"), Type: "float64", Tag: `json:"total"`},
		},
		{
			input:   "Name",
			wantErr: true,
		},
		{
			input:   ":string",
			wantErr: true,
		},
		{
			input:   "Name:",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			field, err := ParseField(test.input)

			if test.wantErr {
				if err == nil {
					t.Fatalf("ParseField(%q) expected an error", test.input)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseField(%q) unexpected error: %s", test.input, err.Error())
			}

			if field != test.expected {
				t.Errorf("ParseField(%q)\n got: %+v\nwant: %+v", test.input, field, test.expected)
			}
		})
	}
}

func TestParseGenType(t *testing.T) {
	tests := map[string]GenType{
		"adapter":   GenAdapter,
		"ADAPTER":   GenAdapter,
		" adapter ": GenAdapter,
		"api":       GenServer,
		"webserver": GenServer,
		"cdk":       GenInfra,
		"vo":        GenValobj,
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			genType, err := ParseGenType(input)
			if err != nil {
				t.Fatalf("ParseGenType(%q) unexpected error: %s", input, err.Error())
			}

			if genType != expected {
				t.Errorf("ParseGenType(%q) = %s, want %s", input, genType, expected)
			}
		})
	}

	if _, err := ParseGenType("widget"); err == nil {
		t.Error("ParseGenType(\"widget\") expected an error")
	}
}
