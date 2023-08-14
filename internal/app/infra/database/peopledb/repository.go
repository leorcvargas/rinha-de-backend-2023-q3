package peopledb

import (
	"database/sql"
	"strings"

	"github.com/leorcvargas/rinha-2023-q3/internal/app/domain/people"
	"github.com/redis/go-redis/v9"
)

type PersonRepository struct {
	db         *sql.DB
	cache      *PeopleDbCache
	mem        *PeopleMemoryStorage
	insertChan chan people.Person
}

func (p *PersonRepository) Create(person *people.Person) (*people.Person, error) {
	nicknameTaken, err := p.cache.GetNickname(person.Nickname)
	if err != nil {
		return nil, err
	}

	if nicknameTaken {
		return nil, people.ErrNicknameTaken
	}

	go func() {
		p.mem.Insert(*person)
	}()

	go func() {
		p.cache.SetNickname(person.Nickname)
		p.cache.Set(person.ID.String(), person)
	}()

	p.insertChan <- *person

	return person, nil
}

func (p *PersonRepository) FindByID(id string) (*people.Person, error) {
	cachedPerson, err := p.cache.Get(id)

	if err != nil && err != redis.Nil {
		return nil, err
	}

	if cachedPerson != nil {
		return cachedPerson, nil
	}

	var person people.Person
	var strStack string

	err = p.db.QueryRow(
		SelectPersonByIDQuery,
		id,
	).Scan(
		&person.ID,
		&person.Nickname,
		&person.Name,
		&person.Birthdate,
		&strStack,
	)
	person.Stack = strings.Split(strStack, ",")

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, people.ErrPersonNotFound
		}

		return nil, err
	}

	return &person, nil
}

func (p *PersonRepository) Search(term string) ([]people.Person, error) {
	memResult := p.mem.Search(term)
	if len(memResult) > 0 {
		return memResult, nil
	}

	result, err := p.searchFts(term)
	if err != nil {
		return nil, err
	}
	if len(result) > 0 {
		return result, nil
	}

	return p.searchTrigram(term)
}

func (p *PersonRepository) searchFts(term string) ([]people.Person, error) {
	rows, err := p.db.Query(
		SearchPeopleFtsQuery,
		term,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result, err := mapSearchResult(rows)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (p *PersonRepository) searchTrigram(term string) ([]people.Person, error) {
	rows, err := p.db.Query(
		SearchPeopleTrgmQuery,
		term,
	)
	if err != nil {
		return nil, err
	}

	return mapSearchResult(rows)
}

func (p *PersonRepository) CountAll() (int64, error) {
	var total int64

	err := p.db.QueryRow(
		CountPeopleQuery,
	).Scan(&total)
	if err != nil {
		return 0, err
	}

	return total, nil
}

func mapSearchResult(rows *sql.Rows) ([]people.Person, error) {
	result := make([]people.Person, 0)
	for rows.Next() {
		var person people.Person
		var strStack string
		var birthdate string

		err := rows.Scan(
			&person.ID,
			&person.Nickname,
			&person.Name,
			&birthdate,
			&strStack,
		)
		if err != nil {
			return nil, err
		}

		person.Stack = strings.Split(strStack, ",")
		person.Birthdate = birthdate[0:10]

		result = append(result, person)
	}

	return result, nil
}

func NewPersonRepository(db *sql.DB, cache *PeopleDbCache, mem *PeopleMemoryStorage) people.Repository {
	insertChan := make(chan people.Person)
	go Worker(insertChan, db)

	return &PersonRepository{
		db:         db,
		cache:      cache,
		insertChan: insertChan,
		mem:        mem,
	}
}
